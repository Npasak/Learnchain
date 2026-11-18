package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"elearning-sbt/config"
	"elearning-sbt/database"
	"elearning-sbt/models"
	"elearning-sbt/version"

	"github.com/gofiber/fiber/v2"
)

// DefaultGASSignalingURL is the built-in lightweight signaling hub for peer/IP discovery and fallback.
const DefaultGASSignalingURL = "https://script.google.com/macros/s/AKfycbx5ttiX6_YQjfY4Oo5O8IplLAX5ow6GnIq3pAS46gCmEWqXUdsZh2W7P8XebGB8Uuuusw/exec"

// isPrivateOrLocalHost returns true for loopback and private/link-local LAN IPs so corporate HTTP proxies are bypassed for LAN sync.
func isPrivateOrLocalHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

var syncHTTPTransport = &http.Transport{
	Proxy: func(req *http.Request) (*url.URL, error) {
		if req != nil && req.URL != nil && isPrivateOrLocalHost(req.URL.Hostname()) {
			return nil, nil
		}
		return http.ProxyFromEnvironment(req)
	},
	DialContext: (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          20,
	IdleConnTimeout:       30 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

func newSyncHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: syncHTTPTransport,
	}
}

// ActiveNodeInfo represents a connected machine in the sync network
type ActiveNodeInfo struct {
	NodeID         string   `json:"node_id"`
	Hostname       string   `json:"hostname"`
	LanURL         string   `json:"lan_url"`
	LanURLs        []string `json:"lan_urls,omitempty"`
	PublicIP       string   `json:"public_ip,omitempty"`
	AppVersion     string   `json:"app_version,omitempty"`
	LastSeenMs     int64    `json:"last_seen_ms"`
	LastSeenText   string   `json:"last_seen_text"`
	UsersCount     int      `json:"users_count"`
	LessonsCount   int      `json:"lessons_count"`
	CertsCount     int      `json:"certs_count"`
	IsSelf         bool     `json:"is_self,omitempty"`
	IsMaster       bool     `json:"is_master"`
	StabilityScore float64  `json:"stability_score"`
	ComputeScore   float64  `json:"compute_score,omitempty"`
	LatencyMs      int64    `json:"latency_ms,omitempty"`
}

// SyncConfig stores persistent Auto-Sync settings for this machine
type SyncConfig struct {
	GasURL               string           `json:"gas_url"`
	AutoSyncEnabled      bool             `json:"auto_sync_enabled"`
	IntervalSec          int              `json:"interval_sec"`
	TimeoutSec           int              `json:"timeout_sec"`
	PreferredMasterIP    string           `json:"preferred_master_ip,omitempty"` // "" = auto fastest IP
	PreferredMasterID    string           `json:"preferred_master_id,omitempty"` // therunrock override
	LastSuccessfulSyncMs int64            `json:"last_successful_sync_ms,omitempty"`
	KnownPeers           []ActiveNodeInfo `json:"known_peers,omitempty"`
}

type SyncHandler struct {
	mu               sync.Mutex
	cfg              SyncConfig
	lastSyncAt       string
	lastSyncStatus   string
	syncTransport    string
	gasStatus        string // "connected" | "standby" | "error"
	gasStatusText    string
	activeNodes      map[string]ActiveNodeInfo
	startedAt        time.Time
	computeScore     float64
	syncSuccessCount int64
	syncFailCount    int64
	isSelfMaster     bool
	electedMasterID  string
	electedMasterIP  string
}

var GlobalSyncHandler *SyncHandler

func NewSyncHandler() *SyncHandler {
	defaultGAS := os.Getenv("GAS_SYNC_URL")
	if strings.TrimSpace(defaultGAS) == "" {
		defaultGAS = DefaultGASSignalingURL
	}
	h := &SyncHandler{
		cfg: SyncConfig{
			GasURL:          defaultGAS,
			AutoSyncEnabled: true,
			IntervalSec:     30,
			TimeoutSec:      180, // 3 minutes default timeout before inactive machine is removed
		},
		activeNodes:   make(map[string]ActiveNodeInfo),
		startedAt:     time.Now(),
		computeScore:  measureComputeScore(),
		syncTransport: "LAN/WiFi Auto",
		gasStatus:     "standby",
		gasStatusText: "รอเชื่อมต่อ GAS Signaling...",
	}
	h.loadConfigFromFile()
	// Migrate legacy .sync.json where a LAN IP was stored in gas_url instead of a GAS URL
	if trimmed := strings.TrimSpace(h.cfg.GasURL); trimmed == "" || !strings.Contains(trimmed, "script.google.com") {
		if trimmed != "" {
			h.cfg.KnownPeers = append(h.cfg.KnownPeers, ActiveNodeInfo{
				NodeID:   trimmed,
				Hostname: "Legacy-Hub",
				LanURL:   strings.TrimRight(trimmed, "/"),
			})
		}
		h.cfg.GasURL = DefaultGASSignalingURL
		h.cfg.AutoSyncEnabled = true
		h.saveConfigToFile()
	}
	h.touchSelfNode()
	GlobalSyncHandler = h
	h.StartAutoSyncWorker()
	return h
}

// measureComputeScore runs a fast ~15ms SHA-256 + math benchmark to score node calculation speed (0..40).
func measureComputeScore() float64 {
	start := time.Now()
	buf := []byte("learnchain-stability-benchmark")
	var ops int
	for time.Since(start) < 15*time.Millisecond {
		sum := sha256.Sum256(buf)
		buf[0] ^= sum[0]
		ops++
	}
	elapsedSec := time.Since(start).Seconds()
	if elapsedSec <= 0 {
		elapsedSec = 0.015
	}
	opsPerMs := float64(ops) / (elapsedSec * 1000.0)
	// Normalize smoothly to 0..40 points
	score := math.Min(40.0, opsPerMs/50.0)
	return math.Round(score*10) / 10
}

// selfStabilityScoreLocked computes overall system stability & performance (0..100).
// Combines calculation speed (40%), network reliability (40%), and uptime stability (20%).
func (h *SyncHandler) selfStabilityScoreLocked() float64 {
	total := h.syncSuccessCount + h.syncFailCount
	reliability := 40.0
	if total > 0 {
		reliability = 40.0 * (float64(h.syncSuccessCount) / float64(total))
	}
	uptimeMin := time.Since(h.startedAt).Minutes()
	uptimeBonus := math.Min(20.0, uptimeMin*2.0)
	totalScore := h.computeScore + reliability + uptimeBonus
	return math.Round(totalScore*10) / 10
}

func (h *SyncHandler) configFilePath() string {
	dbPath := "elearning.db"
	if config.AppConfig != nil && config.AppConfig.DBPath != "" {
		dbPath = config.AppConfig.DBPath
	}
	return dbPath + ".sync.json"
}

func (h *SyncHandler) loadConfigFromFile() {
	data, err := os.ReadFile(h.configFilePath())
	if err != nil {
		return
	}
	var loaded SyncConfig
	if json.Unmarshal(data, &loaded) == nil {
		if loaded.IntervalSec <= 0 {
			loaded.IntervalSec = 30
		}
		if loaded.TimeoutSec <= 0 {
			loaded.TimeoutSec = 180
		}
		h.cfg = loaded
		for _, p := range loaded.KnownPeers {
			if p.NodeID != "" && p.LanURL != "" {
				h.activeNodes[p.NodeID] = p
			}
		}
	}
}

func (h *SyncHandler) saveConfigToFile() {
	data, err := json.MarshalIndent(h.cfg, "", "  ")
	if err == nil {
		_ = os.WriteFile(h.configFilePath(), data, 0644)
	}
}

// FormatHTTPURL formats an http:// URL, omitting :80 when using the standard HTTP port.
func FormatHTTPURL(host, port string) string {
	if port == "" || port == "80" {
		return "http://" + host
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func getMachineIdentity() (nodeID, hostname, primaryLanURL string, lanURLs []string) {
	hostname, _ = os.Hostname()
	if hostname == "" {
		hostname = "Nurse-PC"
	}
	port := "80"
	if config.AppConfig != nil && config.AppConfig.Port != "" {
		port = config.AppConfig.Port
	}
	ips := GetLocalIPs()
	for _, ip := range ips {
		lanURLs = append(lanURLs, FormatHTTPURL(ip, port))
	}
	if len(lanURLs) > 0 {
		primaryLanURL = lanURLs[0]
	} else {
		primaryLanURL = FormatHTTPURL("localhost", port)
	}
	nodeID = fmt.Sprintf("%s:%s", hostname, port)
	return
}

func (h *SyncHandler) touchSelfNode() ActiveNodeInfo {
	nodeID, hostname, primaryLanURL, lanURLs := getMachineIdentity()
	var uCount, lCount, cCount int64
	if database.DB != nil {
		database.DB.Model(&models.User{}).Count(&uCount)
		database.DB.Model(&models.Lesson{}).Count(&lCount)
		database.DB.Model(&models.Certificate{}).Count(&cCount)
	}
	now := time.Now()
	if h.electedMasterID == "" {
		h.electedMasterID = nodeID
		h.isSelfMaster = true
		h.electedMasterIP = primaryLanURL
	}
	publicIPCache.mu.Lock()
	cachedPubIP := publicIPCache.ip
	publicIPCache.mu.Unlock()

	selfNode := ActiveNodeInfo{
		NodeID:         nodeID,
		Hostname:       hostname,
		LanURL:         primaryLanURL,
		LanURLs:        lanURLs,
		PublicIP:       cachedPubIP,
		AppVersion:     version.Version,
		LastSeenMs:     now.UnixMilli(),
		LastSeenText:   now.Format("2006-01-02 15:04:05"),
		UsersCount:     int(uCount),
		LessonsCount:   int(lCount),
		CertsCount:     int(cCount),
		IsSelf:         true,
		IsMaster:       h.isSelfMaster,
		StabilityScore: h.selfStabilityScoreLocked(),
		ComputeScore:   h.computeScore,
	}
	h.activeNodes[nodeID] = selfNode
	return selfNode
}

// pruneExpiredNodesLocked removes any machine that has not checked in within TimeoutSec
func (h *SyncHandler) pruneExpiredNodesLocked() []ActiveNodeInfo {
	nowMs := time.Now().UnixMilli()
	timeoutMs := int64(h.cfg.TimeoutSec) * 1000
	if timeoutMs <= 0 {
		timeoutMs = 180 * 1000
	}
	selfID, _, _, _ := getMachineIdentity()
	var list []ActiveNodeInfo
	for id, node := range h.activeNodes {
		if nowMs-node.LastSeenMs > timeoutMs && id != selfID {
			delete(h.activeNodes, id)
			continue
		}
		node.IsSelf = (id == selfID)
		list = append(list, node)
	}
	return list
}

// StartAutoSyncWorker periodically syncs with Google Apps Script / Hub and prunes inactive machines
func (h *SyncHandler) StartAutoSyncWorker() {
	go func() {
		for {
			time.Sleep(5 * time.Second)
			h.mu.Lock()
			enabled := h.cfg.AutoSyncEnabled
			gasURL := strings.TrimSpace(h.cfg.GasURL)
			interval := h.cfg.IntervalSec
			h.touchSelfNode()
			h.pruneExpiredNodesLocked()
			h.mu.Unlock()

			if interval < 5 {
				interval = 5
			}
			if enabled && gasURL != "" {
				_, _ = h.ExecuteCloudAndPeerAutoSync()
			}
			time.Sleep(time.Duration(interval-5) * time.Second)
		}
	}()
}

// SyncSnapshot represents the complete portable dataset across machines
type SyncSnapshot struct {
	Version             string                          `json:"version"`
	ExportedAt          time.Time                       `json:"exported_at"`
	SourceHost          string                          `json:"source_host"`
	Users               []SyncUserDTO                   `json:"users"`
	Courses             []models.Course                 `json:"courses"`
	Lessons             []SyncLessonDTO                 `json:"lessons"`
	CoverFiles          []SyncCoverFileDTO              `json:"cover_files,omitempty"`
	DeletedCovers       []string                        `json:"deleted_covers,omitempty"`
	DeletedLessonTitles []string                        `json:"deleted_lesson_titles,omitempty"`
	DeletedLessons      []DeletedLessonTombstoneDTO     `json:"deleted_lessons,omitempty"`
	DeletedAssignments  []DeletedAssignmentTombstoneDTO `json:"deleted_assignments,omitempty"`
	DeletedUsers        []DeletedUserTombstoneDTO       `json:"deleted_users,omitempty"`
	ModeratedComments   []ModeratedCommentTombstoneDTO  `json:"moderated_comments,omitempty"`
	Assignments         []SyncAssignmentDTO             `json:"assignments"`
	ProgressList        []SyncProgressDTO               `json:"progress_list"`
	Certificates        []SyncCertificateDTO            `json:"certificates"`
}

type DeletedLessonTombstoneDTO struct {
	Title     string    `json:"title"`
	DeletedAt time.Time `json:"deleted_at"`
}

type DeletedAssignmentTombstoneDTO struct {
	LessonTitle string    `json:"lesson_title"`
	UserEmail   string    `json:"user_email"`
	UserWallet  string    `json:"user_wallet"`
	DeletedAt   time.Time `json:"deleted_at"`
}

type DeletedUserTombstoneDTO struct {
	Email         string    `json:"email"`
	Role          string    `json:"role"`
	WalletAddress string    `json:"wallet_address"`
	DeletedAt     time.Time `json:"deleted_at"`
}

type ModeratedCommentTombstoneDTO struct {
	LessonTitle string    `json:"lesson_title"`
	UserEmail   string    `json:"user_email"`
	UserWallet  string    `json:"user_wallet"`
	DeletedAt   time.Time `json:"deleted_at"`
}

type SyncUserDTO struct {
	WalletAddress string    `json:"wallet_address"`
	Email         string    `json:"email"`
	PasswordHash  string    `json:"password_hash"`
	Name          string    `json:"name"`
	Ward          string    `json:"ward"`
	Role          string    `json:"role"`
	AvatarURL     string    `json:"avatar_url"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type SyncCoverFileDTO struct {
	Filename   string `json:"filename"`
	DataBase64 string `json:"data_base64"`
}

type SyncLessonDTO struct {
	CourseTitle     string        `json:"course_title"`
	SortOrder       int           `json:"sort_order"`
	Title           string        `json:"title"`
	Summary         string        `json:"summary"`
	Description     string        `json:"description"`
	YoutubeURL      string        `json:"youtube_url"`
	ThumbnailURL    string        `json:"thumbnail_url"`
	DocumentURL     string        `json:"document_url"`
	DurationSec     float64       `json:"duration_sec"`
	StudyHours      float64       `json:"study_hours"`
	OpenPeriod      string        `json:"open_period"`
	IsHidden        bool          `json:"is_hidden"`
	PassScorePct    float64       `json:"pass_score_pct"`
	DisablePreTest  bool          `json:"disable_pre_test"`
	DisablePostTest bool          `json:"disable_post_test"`
	UpdatedAt       time.Time     `json:"updated_at,omitempty"`
	Quizzes         []models.Quiz `json:"quizzes"`
}

type SyncAssignmentDTO struct {
	LessonTitle string    `json:"lesson_title"`
	UserEmail   string    `json:"user_email"`
	UserWallet  string    `json:"user_wallet"`
	CreatedAt   time.Time `json:"created_at"`
}

type SyncProgressDTO struct {
	UserEmail      string  `json:"user_email"`
	UserWallet     string  `json:"user_wallet"`
	LessonTitle    string  `json:"lesson_title"`
	LastWatchedSec float64 `json:"last_watched_sec"`
	VideoWatched   bool    `json:"video_watched"`
	PreTestPassed  bool    `json:"pre_test_passed"`
	PostTestPassed bool    `json:"post_test_passed"`
	IsCompleted    bool    `json:"is_completed"`
	OnChainTxHash  string  `json:"on_chain_tx_hash"`
	BestScore      float64 `json:"best_score"`
	TotalScore     float64 `json:"total_score"`
}

type SyncCertificateDTO struct {
	UserEmail         string    `json:"user_email"`
	UserWallet        string    `json:"user_wallet"`
	LessonTitle       string    `json:"lesson_title"`
	StudyHours        float64   `json:"study_hours"`
	LessonDurationSec float64   `json:"lesson_duration_sec"`
	TokenID           string    `json:"token_id"`
	TransactionHash   string    `json:"transaction_hash"`
	LessonsCount      int       `json:"lessons_count"`
	TotalWatchSec     float64   `json:"total_watch_sec"`
	AverageScore      float64   `json:"average_score"`
	IssuedAt          time.Time `json:"issued_at"`
}

// GetLocalIPs returns non-loopback, non-link-local IPv4 addresses of active network interfaces
func GetLocalIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addrs {
			if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && !ipnet.IP.IsLinkLocalUnicast() {
				if ip4 := ipnet.IP.To4(); ip4 != nil {
					ips = append(ips, ip4.String())
				}
			}
		}
	}
	return ips
}

var publicIPCache struct {
	mu        sync.Mutex
	ip        string
	webOnline bool
	latencyMs int64
	checkedAt time.Time
}

func getPublicIPAndWebStatus() (string, bool, int64) {
	publicIPCache.mu.Lock()
	if !publicIPCache.checkedAt.IsZero() && time.Since(publicIPCache.checkedAt) < 5*time.Minute {
		ip, online, lat := publicIPCache.ip, publicIPCache.webOnline, publicIPCache.latencyMs
		publicIPCache.mu.Unlock()
		return ip, online, lat
	}
	publicIPCache.mu.Unlock()

	client := newSyncHTTPClient(2500 * time.Millisecond)
	start := time.Now()
	ipOut := ""
	online := false
	resp, err := client.Get("https://api.ipify.org")
	lat := time.Since(start).Milliseconds()
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if b, rErr := io.ReadAll(io.LimitReader(resp.Body, 64)); rErr == nil {
				candidate := strings.TrimSpace(string(b))
				if net.ParseIP(candidate) != nil {
					ipOut = candidate
					online = true
				}
			}
		}
	}

	publicIPCache.mu.Lock()
	publicIPCache.ip = ipOut
	publicIPCache.webOnline = online
	publicIPCache.latencyMs = lat
	publicIPCache.checkedAt = time.Now()
	publicIPCache.mu.Unlock()

	return ipOut, online, lat
}

// Ping returns lightweight node status & stability score for fastest-IP benchmarking.
func (h *SyncHandler) Ping(c *fiber.Ctx) error {
	nodeID, hostname, primaryLanURL, _ := getMachineIdentity()
	h.mu.Lock()
	score := h.selfStabilityScoreLocked()
	isMaster := h.isSelfMaster
	h.mu.Unlock()
	return c.JSON(fiber.Map{
		"ok":              true,
		"node_id":         nodeID,
		"hostname":        hostname,
		"lan_url":         primaryLanURL,
		"stability_score": score,
		"is_master":       isMaster,
		"server_time_ms":  time.Now().UnixMilli(),
	})
}

// GetSyncInfo returns network URLs, QR info, Auto-Sync config, active machines, and record counts
func (h *SyncHandler) GetSyncInfo(c *fiber.Ctx) error {
	isGod := requireGodTherunrock(c)
	nodeID, hostname, primaryLanURL, lanURLs := getMachineIdentity()
	port := "80"
	if config.AppConfig != nil && config.AppConfig.Port != "" {
		port = config.AppConfig.Port
	}

	var userCount, lessonCount, assignCount, progressCount, certCount int64
	database.DB.Model(&models.User{}).Count(&userCount)
	database.DB.Model(&models.Lesson{}).Count(&lessonCount)
	database.DB.Model(&models.LessonAssignment{}).Count(&assignCount)
	database.DB.Model(&models.UserLessonProgress{}).Count(&progressCount)
	database.DB.Model(&models.Certificate{}).Count(&certCount)

	h.mu.Lock()
	h.touchSelfNode()
	nodes := h.pruneExpiredNodesLocked()
	cfgCopy := h.cfg
	if !isGod {
		cfgCopy.GasURL = ""
	}
	lastSyncAt := h.lastSyncAt
	lastSyncStatus := h.lastSyncStatus
	syncTransport := h.syncTransport
	gasStatus := h.gasStatus
	gasStatusText := h.gasStatusText
	masterID := h.electedMasterID
	masterIP := h.electedMasterIP
	isSelfMaster := h.isSelfMaster
	syncSuccessCount := h.syncSuccessCount
	syncFailCount := h.syncFailCount
	lastSuccessMs := h.cfg.LastSuccessfulSyncMs
	localScore := h.selfStabilityScoreLocked()
	h.mu.Unlock()

	offlineDays := 0
	staleOfflineWarning := false
	if lastSuccessMs > 0 {
		offlineDays = int(time.Since(time.UnixMilli(lastSuccessMs)).Hours() / 24)
		if offlineDays >= 30 {
			staleOfflineWarning = true
		}
	}

	publicIP, webOnline, webLatencyMs := getPublicIPAndWebStatus()

	return c.JSON(fiber.Map{
		"success":                 true,
		"app_version":             version.Version,
		"is_god_therunrock":       isGod,
		"gas_url_hidden":          !isGod,
		"node_id":                 nodeID,
		"hostname":                hostname,
		"port":                    port,
		"primary_lan_url":         primaryLanURL,
		"lan_ips":                 GetLocalIPs(),
		"lan_urls":                lanURLs,
		"public_ip":               publicIP,
		"web_online":              webOnline,
		"web_latency_ms":          webLatencyMs,
		"is_self_master":          isSelfMaster,
		"elected_master_id":       masterID,
		"elected_master_ip":       masterIP,
		"sync_transport":          syncTransport,
		"sync_success_count":      syncSuccessCount,
		"sync_fail_count":         syncFailCount,
		"last_successful_sync_ms": lastSuccessMs,
		"offline_days":            offlineDays,
		"stale_offline_warning":   staleOfflineWarning,
		"gas_status":            gasStatus,
		"gas_status_text":       gasStatusText,
		"local_stability_score": localScore,
		"user_count":            userCount,
		"lesson_count":          lessonCount,
		"progress_count":        progressCount,
		"cert_count":            certCount,
		"counts": fiber.Map{
			"users":        userCount,
			"lessons":      lessonCount,
			"assignments":  assignCount,
			"progress":     progressCount,
			"certificates": certCount,
		},
		"config":           cfgCopy,
		"last_sync_at":     lastSyncAt,
		"last_sync_status": lastSyncStatus,
		"online_count":     len(nodes),
		"active_nodes":     nodes,
	})
}

// UpdateSyncConfig saves Auto-Sync settings (GAS URL & Master override restricted to therunrock)
func (h *SyncHandler) UpdateSyncConfig(c *fiber.Ctx) error {
	isGod := requireGodTherunrock(c)
	var req SyncConfig
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "ข้อมูลตั้งค่าไม่ถูกต้อง",
		})
	}
	if req.IntervalSec < 5 {
		req.IntervalSec = 30
	}
	if req.TimeoutSec < 10 {
		req.TimeoutSec = 180
	}

	h.mu.Lock()
	if isGod {
		if trimmed := strings.TrimSpace(req.GasURL); trimmed != "" {
			h.cfg.GasURL = trimmed
		}
		h.cfg.PreferredMasterID = strings.TrimSpace(req.PreferredMasterID)
	}
	h.cfg.PreferredMasterIP = strings.TrimSpace(req.PreferredMasterIP)
	h.cfg.AutoSyncEnabled = req.AutoSyncEnabled
	h.cfg.IntervalSec = req.IntervalSec
	h.cfg.TimeoutSec = req.TimeoutSec
	h.saveConfigToFile()
	cfgCopy := h.cfg
	if !isGod {
		cfgCopy.GasURL = ""
	}
	h.mu.Unlock()

	if h.cfg.AutoSyncEnabled {
		resMap, err := h.ExecuteCloudAndPeerAutoSync()
		if err != nil {
			return c.JSON(fiber.Map{
				"success": false,
				"error":   err.Error(),
				"config":  cfgCopy,
			})
		}
		return c.JSON(resMap)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "บันทึกการตั้งค่า Auto-Sync เรียบร้อยแล้ว",
		"config":  cfgCopy,
	})
}

// TriggerAutoSyncNow forces an immediate sync cycle using LAN/WiFi primary or GAS fallback
func (h *SyncHandler) TriggerAutoSyncNow(c *fiber.Ctx) error {
	isGod := requireGodTherunrock(c)
	var body struct {
		GasURL            string `json:"gas_url"`
		TimeoutSec        int    `json:"timeout_sec"`
		PreferredMasterIP string `json:"preferred_master_ip"`
		PreferredMasterID string `json:"preferred_master_id"`
	}
	_ = c.BodyParser(&body)

	h.mu.Lock()
	if isGod && strings.TrimSpace(body.GasURL) != "" {
		h.cfg.GasURL = strings.TrimSpace(body.GasURL)
	}
	if isGod && body.PreferredMasterID != "" {
		if body.PreferredMasterID == "auto" {
			h.cfg.PreferredMasterID = ""
		} else {
			h.cfg.PreferredMasterID = strings.TrimSpace(body.PreferredMasterID)
		}
	}
	if body.PreferredMasterIP != "" {
		if body.PreferredMasterIP == "auto" {
			h.cfg.PreferredMasterIP = ""
		} else {
			h.cfg.PreferredMasterIP = strings.TrimSpace(body.PreferredMasterIP)
		}
	}
	if body.TimeoutSec >= 1 {
		h.cfg.TimeoutSec = body.TimeoutSec
	}
	h.saveConfigToFile()
	h.mu.Unlock()

	resMap, err := h.ExecuteCloudAndPeerAutoSync()
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}
	return c.JSON(resMap)
}

// ExecuteCloudAndPeerAutoSync follows the hybrid signaling & direct-LAN rules:
// 1. Use GAS only as lightweight signaling (Peer, URL, IP, LocalIP, StabilityScore) when LAN/WiFi peers are reachable.
// 2. Elect exactly 1 Master based on overall stability & performance score (or therunrock override) and auto-pick the fastest IP.
// 3. Transfer main snapshot directly via LAN/WiFi/LocalIP; automatically fall back to GAS full-sync ONLY if no LAN/WiFi peer is reachable.
func (h *SyncHandler) ExecuteCloudAndPeerAutoSync() (fiber.Map, error) {
	h.mu.Lock()
	targetURL := strings.TrimSpace(h.cfg.GasURL)
	timeoutSec := h.cfg.TimeoutSec
	preferredIP := strings.TrimSpace(h.cfg.PreferredMasterIP)
	preferredMasterID := strings.TrimSpace(h.cfg.PreferredMasterID)
	selfScore := h.selfStabilityScoreLocked()
	isSelfMaster := h.isSelfMaster
	h.mu.Unlock()

	nodeID, hostname, primaryLanURL, lanURLs := getMachineIdentity()
	localSnap := BuildSnapshot()

	// Step A: Lightweight GAS Signaling (discover peers/IPs/scores without sending heavy snapshot)
	gasEndpoint := targetURL
	if !strings.Contains(gasEndpoint, "script.google.com") {
		gasEndpoint = DefaultGASSignalingURL
	}
	var signalOK bool
	if gasEndpoint != "" {
		signalNodes, sigErr := h.sendGASSignal(gasEndpoint, nodeID, hostname, primaryLanURL, lanURLs, timeoutSec, isSelfMaster, selfScore, localSnap)
		if sigErr == nil {
			signalOK = true
			h.mu.Lock()
			h.gasStatus = "connected"
			h.gasStatusText = "เชื่อมต่อ GAS Signaling ปกติ"
			for _, n := range signalNodes {
				if n.NodeID != "" && n.NodeID != nodeID {
					n.IsSelf = false
					h.activeNodes[n.NodeID] = n
				}
			}
			h.mu.Unlock()
		} else {
			h.mu.Lock()
			h.gasStatus = "error"
			h.gasStatusText = "ไม่สามารถเชื่อมต่อ GAS Signaling ได้"
			h.mu.Unlock()
		}
	}

	// Step B: Elect 1 single Master based on overall stability/performance score (resolves multi-Master conflicts)
	h.mu.Lock()
	h.touchSelfNode()
	allNodes := h.pruneExpiredNodesLocked()
	electedMaster := h.electSingleMasterLocked(allNodes, preferredMasterID)
	selfIsMasterNow := h.isSelfMaster
	hasKnownPeers := len(h.cfg.KnownPeers) > 0
	h.mu.Unlock()

	var hubResp hubSyncResponse
	var usedTransport string
	var chosenMasterIP string
	var chosenMasterLatency int64

	// Step C: Direct LAN/WiFi/LocalIP transfer first (benchmarking fastest IP or using user's manual IP)
	pubIP, _, _ := getPublicIPAndWebStatus()
	payload := fiber.Map{
		"action":          "auto_sync",
		"node_id":         nodeID,
		"hostname":        hostname,
		"lan_url":         primaryLanURL,
		"lan_urls":        lanURLs,
		"public_ip":       pubIP,
		"app_version":     version.Version,
		"is_master":       h.isSelfMaster,
		"stability_score": selfScore,
		"timeout_sec":     timeoutSec,
		"snapshot":        localSnap,
	}
	payloadBytes, _ := json.Marshal(payload)

	// Collect candidate direct LAN targets: elected Master's URLs first, then any other peer
	var directSuccess bool
	if electedMaster != nil && electedMaster.NodeID != nodeID {
		candURLs := collectNodeURLs(*electedMaster)
		bestURL, latMs, ok := h.selectBestNodeURL(candURLs, preferredIP)
		if ok {
			if respData, err := postSyncPayload(strings.TrimRight(bestURL, "/")+"/api/sync/gas-hub", payloadBytes, 12*time.Second); err == nil {
				hubResp = *respData
				directSuccess = true
				usedTransport = fmt.Sprintf("LAN/WiFi Direct (%s • %dms)", bestURL, latMs)
				chosenMasterIP = bestURL
				chosenMasterLatency = latMs
			}
		}
	}

	// If elected Master wasn't reachable directly (or self was Master), try any other reachable LAN peer
	if !directSuccess {
		fbResp, fbPeer, fbURL, fbLat, fbErr := h.tryFastestLANPeerSync(payloadBytes, nodeID, primaryLanURL, preferredIP)
		if fbErr == nil {
			hubResp = *fbResp
			directSuccess = true
			usedTransport = fmt.Sprintf("LAN/WiFi Direct (%s • %dms)", fbURL, fbLat)
			chosenMasterIP = fbURL
			chosenMasterLatency = fbLat
			if fbPeer != nil {
				h.mu.Lock()
				h.electedMasterID = fbPeer.NodeID
				h.electedMasterIP = fbURL
				h.mu.Unlock()
			}
		}
	}

	// Step D: If this machine IS the Master and there are no other online/known peers yet,
	// Master simply stays ready on LAN/WiFi and keeps lightweight GAS Signaling active (no heavy GAS fallback upload!).
	// If other peers exist but direct LAN transfer failed (e.g. corporate firewall/VLAN), fall through to Step E (GAS Fallback) so Master & Peers can exchange snapshots.
	if !directSuccess && selfIsMasterNow && len(allNodes) <= 1 && !hasKnownPeers {
		h.mu.Lock()
		h.touchSelfNode()
		activeList := h.pruneExpiredNodesLocked()
		h.updateKnownPeersLocked(activeList)
		if signalOK {
			h.syncSuccessCount++
			h.syncTransport = "Master Ready (LAN/WiFi + GAS Signaling)"
			h.lastSyncAt = time.Now().Format("15:04:05")
			h.lastSyncStatus = fmt.Sprintf("✅ Master พร้อมรับการเชื่อมต่อ (GAS Signaling ปกติ • ออนไลน์ %d เครื่อง)", len(activeList))
		} else {
			h.syncTransport = "Master Ready (LAN/WiFi Only)"
			h.lastSyncAt = time.Now().Format("15:04:05")
			h.lastSyncStatus = fmt.Sprintf("🟡 Master พร้อมรับการเชื่อมต่อบน LAN (GAS ไม่ตอบสนอง • ออนไลน์ %d เครื่อง)", len(activeList))
		}
		h.mu.Unlock()

		return fiber.Map{
			"success":          true,
			"message":          h.lastSyncStatus,
			"online_count":     len(activeList),
			"active_nodes":     activeList,
			"last_sync_at":     h.lastSyncAt,
			"last_sync_status": h.lastSyncStatus,
		}, nil
	}

	// Step E: Automatic GAS Fallback ONLY for Peers when direct LAN/WiFi/IP transfer failed
	if !directSuccess {
		fallbackURL := gasEndpoint
		if fallbackURL == "" {
			fallbackURL = DefaultGASSignalingURL
		}

		respData, err := postSyncPayload(fallbackURL, payloadBytes, 60*time.Second)
		if err != nil {
			h.mu.Lock()
			h.syncFailCount++
			h.gasStatus = "error"
			h.gasStatusText = "เชื่อมต่อ GAS ไม่สำเร็จ"
			h.lastSyncAt = time.Now().Format("15:04:05")
			h.lastSyncStatus = "❌ เชื่อมต่อ LAN และ GAS Fallback ไม่สำเร็จ: " + err.Error()
			h.syncTransport = "Offline"
			h.mu.Unlock()
			return nil, fmt.Errorf("เชื่อมต่อ LAN และ GAS Fallback ไม่สำเร็จ: %v", err)
		}
		hubResp = *respData
		usedTransport = "GAS Fallback (อัตโนมัติ)"
		h.mu.Lock()
		h.gasStatus = "connected"
		h.gasStatusText = "เชื่อมต่อผ่าน GAS Fallback ปกติ"
		h.mu.Unlock()
	}

	// Merge snapshot into local DB
	var mergedStats fiber.Map
	if hubResp.Snapshot != nil {
		mergedStats = MergeSnapshot(hubResp.Snapshot)
	}

	// Update active nodes, re-elect single Master, and persist known peers
	h.mu.Lock()
	h.syncSuccessCount++
	h.activeNodes = make(map[string]ActiveNodeInfo)
	for _, n := range hubResp.ActiveNodes {
		n.IsSelf = (n.NodeID == nodeID)
		if n.NodeID == chosenMasterIP || (electedMaster != nil && n.NodeID == electedMaster.NodeID && chosenMasterLatency > 0) {
			n.LatencyMs = chosenMasterLatency
		}
		h.activeNodes[n.NodeID] = n
	}
	h.touchSelfNode()
	activeList := h.pruneExpiredNodesLocked()
	finalMaster := h.electSingleMasterLocked(activeList, preferredMasterID)
	if chosenMasterIP != "" {
		h.electedMasterIP = chosenMasterIP
	} else if finalMaster != nil {
		h.electedMasterIP = finalMaster.LanURL
	}
	h.updateKnownPeersLocked(activeList)
	h.syncTransport = usedTransport
	h.lastSyncAt = time.Now().Format("15:04:05")
	h.lastSyncStatus = fmt.Sprintf("✅ %s (ออนไลน์ %d เครื่อง)", usedTransport, len(activeList))
	h.mu.Unlock()

	// Broadcast snapshot directly to other active LAN peers
	for _, peer := range activeList {
		if !peer.IsSelf && peer.LanURL != "" && peer.LanURL != primaryLanURL && peer.LanURL != chosenMasterIP {
			go func(peerLanURL string) {
				pClient := newSyncHTTPClient(6 * time.Second)
				snap := BuildSnapshot()
				b, _ := json.Marshal(snap)
				r, e := pClient.Post(strings.TrimRight(peerLanURL, "/")+"/api/sync/import", "application/json", bytes.NewReader(b))
				if e == nil && r != nil {
					r.Body.Close()
				}
			}(peer.LanURL)
		}
	}

	log.Printf("🔄 [Auto-Sync] %s | ออนไลน์ %d เครื่อง", usedTransport, len(activeList))

	return fiber.Map{
		"success":          true,
		"message":          fmt.Sprintf("✅ Auto-Sync สำเร็จ! เชื่อมต่อออนไลน์อยู่ %d เครื่อง", len(activeList)),
		"online_count":     len(activeList),
		"active_nodes":     activeList,
		"pulled_stats":     mergedStats,
		"last_sync_at":     h.lastSyncAt,
		"last_sync_status": h.lastSyncStatus,
	}, nil
}

type hubSyncResponse struct {
	Success     bool             `json:"success"`
	Error       string           `json:"error"`
	OnlineCount int              `json:"online_count"`
	ActiveNodes []ActiveNodeInfo `json:"active_nodes"`
	Snapshot    *SyncSnapshot    `json:"snapshot"`
}

// updateKnownPeersLocked persists non-self peers into SyncConfig so Failover works across restarts.
func (h *SyncHandler) updateKnownPeersLocked(activeList []ActiveNodeInfo) {
	var peers []ActiveNodeInfo
	for _, n := range activeList {
		if !n.IsSelf && n.LanURL != "" {
			peers = append(peers, n)
		}
	}
	h.cfg.KnownPeers = peers
	h.cfg.LastSuccessfulSyncMs = time.Now().UnixMilli()
	h.saveConfigToFile()
}

// sendGASSignal sends lightweight peer/IP/score metadata ONLY to GAS (no heavy snapshot)
// to reduce load on GAS while discovering peers and IPs across networks.
func (h *SyncHandler) sendGASSignal(gasURL, nodeID, hostname, primaryLanURL string, lanURLs []string, timeoutSec int, isMaster bool, stabilityScore float64, localSnap SyncSnapshot) ([]ActiveNodeInfo, error) {
	pubIP, _, _ := getPublicIPAndWebStatus()
	sigPayload := fiber.Map{
		"action":          "signal",
		"node_id":         nodeID,
		"hostname":        hostname,
		"lan_url":         primaryLanURL,
		"lan_urls":        lanURLs,
		"public_ip":       pubIP,
		"app_version":     version.Version,
		"is_master":       isMaster,
		"stability_score": stabilityScore,
		"timeout_sec":     timeoutSec,
		"users_count":     len(localSnap.Users),
		"lessons_count":   len(localSnap.Lessons),
		"certs_count":     len(localSnap.Certificates),
	}
	b, _ := json.Marshal(sigPayload)
	client := newSyncHTTPClient(15 * time.Second)
	resp, err := client.Post(gasURL, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed hubSyncResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil || !parsed.Success {
		return nil, fmt.Errorf("invalid GAS signal response")
	}
	return parsed.ActiveNodes, nil
}

func postSyncPayload(endpoint string, payloadBytes []byte, timeout time.Duration) (*hubSyncResponse, error) {
	client := newSyncHTTPClient(timeout)
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("อ่านข้อมูลตอบกลับไม่สำเร็จ: %v", err)
	}
	var parsed hubSyncResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil || !parsed.Success {
		errMsg := parsed.Error
		if errMsg == "" {
			errMsg = "รูปแบบข้อมูลตอบกลับไม่ถูกต้อง"
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	return &parsed, nil
}

func collectNodeURLs(n ActiveNodeInfo) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(u string) {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	add(n.LanURL)
	for _, u := range n.LanURLs {
		add(u)
	}
	return out
}

// selectBestNodeURL probes all candidate IPs concurrently and picks the fastest reachable IP
// (or honors preferredIP if manually selected by the user and reachable).
func (h *SyncHandler) selectBestNodeURL(urls []string, preferredIP string) (string, int64, bool) {
	if len(urls) == 0 {
		return "", 0, false
	}
	type probeRes struct {
		url string
		ms  int64
		ok  bool
	}
	ch := make(chan probeRes, len(urls))
	for _, u := range urls {
		go func(target string) {
			client := newSyncHTTPClient(2500 * time.Millisecond)
			start := time.Now()
			resp, err := client.Get(strings.TrimRight(target, "/") + "/api/sync/ping")
			if err != nil {
				ch <- probeRes{url: target, ok: false}
				return
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ch <- probeRes{url: target, ms: time.Since(start).Milliseconds(), ok: true}
			} else {
				ch <- probeRes{url: target, ok: false}
			}
		}(u)
	}

	var reachable []probeRes
	for i := 0; i < len(urls); i++ {
		r := <-ch
		if r.ok {
			reachable = append(reachable, r)
		}
	}
	if len(reachable) == 0 {
		return "", 0, false
	}

	// If user manually selected a specific IP and it is reachable, honor it
	if preferredIP != "" {
		normPref := strings.TrimRight(preferredIP, "/")
		for _, r := range reachable {
			if strings.EqualFold(r.url, normPref) || strings.Contains(r.url, normPref) {
				return r.url, r.ms, true
			}
		}
	}

	// Otherwise auto-pick the fastest (lowest latency) IP
	sort.Slice(reachable, func(i, j int) bool {
		return reachable[i].ms < reachable[j].ms
	})
	return reachable[0].url, reachable[0].ms, true
}

// electSingleMasterLocked resolves multi-Master conflicts by electing exactly 1 Master
// based on overall stability & performance score (or therunrock manual override).
func (h *SyncHandler) electSingleMasterLocked(nodes []ActiveNodeInfo, preferredMasterID string) *ActiveNodeInfo {
	if len(nodes) == 0 {
		return nil
	}
	selfID, _, _, _ := getMachineIdentity()

	if preferredMasterID != "" {
		for i := range nodes {
			if nodes[i].NodeID == preferredMasterID {
				nodes[i].IsMaster = true
				h.electedMasterID = nodes[i].NodeID
				h.isSelfMaster = (nodes[i].NodeID == selfID)
				return &nodes[i]
			}
		}
	}

	// Sort all online nodes by StabilityScore DESC, then LatencyMs ASC, then NodeID ASC (deterministic)
	sorted := make([]ActiveNodeInfo, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].StabilityScore != sorted[j].StabilityScore {
			return sorted[i].StabilityScore > sorted[j].StabilityScore
		}
		if sorted[i].LatencyMs != sorted[j].LatencyMs && sorted[i].LatencyMs > 0 && sorted[j].LatencyMs > 0 {
			return sorted[i].LatencyMs < sorted[j].LatencyMs
		}
		return sorted[i].NodeID < sorted[j].NodeID
	})

	winner := sorted[0]
	h.electedMasterID = winner.NodeID
	h.isSelfMaster = (winner.NodeID == selfID)
	for i := range nodes {
		nodes[i].IsMaster = (nodes[i].NodeID == winner.NodeID)
		if existing, ok := h.activeNodes[nodes[i].NodeID]; ok {
			existing.IsMaster = nodes[i].IsMaster
			h.activeNodes[nodes[i].NodeID] = existing
		}
	}
	return &winner
}

// tryFastestLANPeerSync finds any reachable LAN/WiFi peer, picks its fastest IP, and syncs directly.
func (h *SyncHandler) tryFastestLANPeerSync(payloadBytes []byte, selfNodeID, selfLanURL, preferredIP string) (*hubSyncResponse, *ActiveNodeInfo, string, int64, error) {
	h.mu.Lock()
	seen := make(map[string]ActiveNodeInfo)
	for _, n := range h.activeNodes {
		if n.NodeID != "" && n.NodeID != selfNodeID && n.LanURL != "" && n.LanURL != selfLanURL {
			seen[n.NodeID] = n
		}
	}
	for _, n := range h.cfg.KnownPeers {
		if n.NodeID != "" && n.NodeID != selfNodeID && n.LanURL != "" && n.LanURL != selfLanURL {
			if _, exists := seen[n.NodeID]; !exists {
				seen[n.NodeID] = n
			}
		}
	}
	h.mu.Unlock()

	if len(seen) == 0 {
		return nil, nil, "", 0, fmt.Errorf("no known LAN peers")
	}

	var candidates []ActiveNodeInfo
	for _, n := range seen {
		candidates = append(candidates, n)
	}
	// Prefer highest stability score, then deterministic NodeID
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].StabilityScore != candidates[j].StabilityScore {
			return candidates[i].StabilityScore > candidates[j].StabilityScore
		}
		return candidates[i].NodeID < candidates[j].NodeID
	})

	for i := range candidates {
		cand := candidates[i]
		bestURL, latMs, ok := h.selectBestNodeURL(collectNodeURLs(cand), preferredIP)
		if !ok {
			continue
		}
		parsed, err := postSyncPayload(strings.TrimRight(bestURL, "/")+"/api/sync/gas-hub", payloadBytes, 8*time.Second)
		if err == nil {
			log.Printf("⚡ [LAN/WiFi] ซิงค์ตรงผ่าน %s (%s • %dms)", cand.Hostname, bestURL, latMs)
			return parsed, &cand, bestURL, latMs, nil
		}
	}

	return nil, nil, "", 0, fmt.Errorf("all LAN/WiFi peers unreachable")
}

// GasHubCompatibleEndpoint handles POST/GET /api/sync/gas-hub with the exact same logic as Google Apps Script:
// - Registers the calling machine's IP/URL and timestamp
// - Deletes any machine that hasn't checked in within TimeoutSec
// - Merges the incoming snapshot into DB and returns the full merged snapshot + active_nodes list
func (h *SyncHandler) GasHubCompatibleEndpoint(c *fiber.Ctx) error {
	var body struct {
		Action         string        `json:"action"`
		NodeID         string        `json:"node_id"`
		Hostname       string        `json:"hostname"`
		LanURL         string        `json:"lan_url"`
		LanURLs        []string      `json:"lan_urls"`
		PublicIP       string        `json:"public_ip"`
		AppVersion     string        `json:"app_version"`
		IsMaster       bool          `json:"is_master"`
		StabilityScore float64       `json:"stability_score"`
		TimeoutSec     int           `json:"timeout_sec"`
		Snapshot       *SyncSnapshot `json:"snapshot"`
	}
	_ = c.BodyParser(&body)

	var stats fiber.Map
	if body.Snapshot != nil {
		stats = MergeSnapshot(body.Snapshot)
	}

	h.mu.Lock()
	if body.TimeoutSec > 0 {
		h.cfg.TimeoutSec = body.TimeoutSec
	}
	h.touchSelfNode()
	now := time.Now()
	if body.Action == "disconnect" && body.NodeID != "" {
		delete(h.activeNodes, body.NodeID)
	} else if body.NodeID != "" {
		uCnt, lCnt, cCnt := 0, 0, 0
		if body.Snapshot != nil {
			uCnt = len(body.Snapshot.Users)
			lCnt = len(body.Snapshot.Lessons)
			cCnt = len(body.Snapshot.Certificates)
		}
		h.activeNodes[body.NodeID] = ActiveNodeInfo{
			NodeID:         body.NodeID,
			Hostname:       body.Hostname,
			LanURL:         body.LanURL,
			LanURLs:        body.LanURLs,
			PublicIP:       body.PublicIP,
			AppVersion:     body.AppVersion,
			IsMaster:       body.IsMaster,
			StabilityScore: body.StabilityScore,
			LastSeenMs:     now.UnixMilli(),
			LastSeenText:   now.Format("2006-01-02 15:04:05"),
			UsersCount:     uCnt,
			LessonsCount:   lCnt,
			CertsCount:     cCnt,
		}
	}
	activeList := h.pruneExpiredNodesLocked()
	h.electSingleMasterLocked(activeList, h.cfg.PreferredMasterID)
	h.updateKnownPeersLocked(activeList)
	h.mu.Unlock()

	mergedSnap := BuildSnapshot()
	return c.JSON(fiber.Map{
		"success":      true,
		"server_time":  now.Format(time.RFC3339),
		"online_count": len(activeList),
		"active_nodes": activeList,
		"stats":        stats,
		"snapshot":     mergedSnap,
	})
}

// GetGasScriptSource returns the ready-to-copy Google Apps Script code
func (h *SyncHandler) GetGasScriptSource(c *fiber.Ctx) error {
	data, err := os.ReadFile("google_apps_script_sync.gs")
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   "ไม่พบไฟล์ google_apps_script_sync.gs",
		})
	}
	return c.JSON(fiber.Map{
		"success": true,
		"script":  string(data),
	})
}

// BuildSnapshot constructs a complete SyncSnapshot from the local SQLite database
func BuildSnapshot() SyncSnapshot {
	hostname, _ := os.Hostname()
	snap := SyncSnapshot{
		Version:    "1.0",
		ExportedAt: time.Now(),
		SourceHost: hostname,
	}

	var users []models.User
	database.DB.Find(&users)
	for _, u := range users {
		snap.Users = append(snap.Users, SyncUserDTO{
			WalletAddress: u.WalletAddress,
			Email:         u.Email,
			PasswordHash:  u.PasswordHash,
			Name:          u.Name,
			Ward:          u.Ward,
			Role:          u.Role,
			AvatarURL:     u.AvatarURL,
			CreatedAt:     u.CreatedAt,
			UpdatedAt:     u.UpdatedAt,
		})
	}

	database.DB.Find(&snap.Courses)

	var lessons []models.Lesson
	database.DB.Preload("Quizzes.Questions.Options").Order("sort_order asc, id asc").Find(&lessons)
	courseMap := make(map[uint]string)
	activeCoverSet := make(map[string]bool)
	seenCoverFiles := make(map[string]bool)
	collectCover := func(thumbURL string) {
		if !strings.HasPrefix(thumbURL, "/covers/") {
			return
		}
		activeCoverSet[thumbURL] = true
		fname := filepath.Base(strings.TrimPrefix(thumbURL, "/covers/"))
		if fname == "" || fname == "." || fname == ".." || seenCoverFiles[fname] {
			return
		}
		if rawBytes, err := os.ReadFile(filepath.Join(coversDir, fname)); err == nil && len(rawBytes) > 0 {
			seenCoverFiles[fname] = true
			snap.CoverFiles = append(snap.CoverFiles, SyncCoverFileDTO{
				Filename:   fname,
				DataBase64: base64.StdEncoding.EncodeToString(rawBytes),
			})
		}
	}
	for _, c := range snap.Courses {
		courseMap[c.ID] = c.Title
		collectCover(c.ThumbnailURL)
	}
	for _, l := range lessons {
		snap.Lessons = append(snap.Lessons, SyncLessonDTO{
			CourseTitle:     courseMap[l.CourseID],
			SortOrder:       l.SortOrder,
			Title:           l.Title,
			Summary:         l.Summary,
			Description:     l.Description,
			YoutubeURL:      l.YoutubeURL,
			ThumbnailURL:    l.ThumbnailURL,
			DocumentURL:     l.DocumentURL,
			DurationSec:     l.DurationSec,
			StudyHours:      l.StudyHours,
			OpenPeriod:      l.OpenPeriod,
			IsHidden:        l.IsHidden,
			PassScorePct:    l.PassScorePct,
			DisablePreTest:  l.DisablePreTest,
			DisablePostTest: l.DisablePostTest,
			UpdatedAt:       l.UpdatedAt,
			Quizzes:         l.Quizzes,
		})
		collectCover(l.ThumbnailURL)
	}

	tombstoneCutoff := time.Now().Add(-90 * 24 * time.Hour)

	var deletedLessons []models.Lesson
	database.DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&deletedLessons)
	seenDeletedCovers := make(map[string]bool)
	seenDeletedTitles := make(map[string]bool)
	for _, retired := range database.RetiredDemoLessonTitles {
		if !seenDeletedTitles[retired] {
			seenDeletedTitles[retired] = true
			snap.DeletedLessonTitles = append(snap.DeletedLessonTitles, retired)
		}
	}
	for _, dl := range deletedLessons {
		if dl.Title != "" && !seenDeletedTitles[dl.Title] {
			seenDeletedTitles[dl.Title] = true
			snap.DeletedLessonTitles = append(snap.DeletedLessonTitles, dl.Title)
		}
		if dl.Title != "" && dl.DeletedAt.Valid && dl.DeletedAt.Time.After(tombstoneCutoff) {
			snap.DeletedLessons = append(snap.DeletedLessons, DeletedLessonTombstoneDTO{
				Title:     dl.Title,
				DeletedAt: dl.DeletedAt.Time,
			})
		}
		if strings.HasPrefix(dl.ThumbnailURL, "/covers/") && !activeCoverSet[dl.ThumbnailURL] && !seenDeletedCovers[dl.ThumbnailURL] {
			seenDeletedCovers[dl.ThumbnailURL] = true
			snap.DeletedCovers = append(snap.DeletedCovers, dl.ThumbnailURL)
		}
	}

	// Tombstones for deleted users (90-day TTL)
	var deletedUsers []models.User
	database.DB.Unscoped().Where("deleted_at IS NOT NULL AND deleted_at >= ?", tombstoneCutoff).Find(&deletedUsers)
	userByIDAll := make(map[uint]models.User)
	for _, u := range users {
		userByIDAll[u.ID] = u
	}
	for _, du := range deletedUsers {
		userByIDAll[du.ID] = du
		if du.DeletedAt.Valid {
			snap.DeletedUsers = append(snap.DeletedUsers, DeletedUserTombstoneDTO{
				Email:         du.Email,
				Role:          du.Role,
				WalletAddress: du.WalletAddress,
				DeletedAt:     du.DeletedAt.Time,
			})
		}
	}

	// Tombstones for deleted/unassigned lesson assignments (90-day TTL)
	var deletedAssigns []models.LessonAssignment
	database.DB.Unscoped().Where("deleted_at IS NOT NULL AND deleted_at >= ?", tombstoneCutoff).Find(&deletedAssigns)
	lessonByIDAll := make(map[uint]models.Lesson)
	for _, l := range lessons {
		lessonByIDAll[l.ID] = l
	}
	for _, dl := range deletedLessons {
		lessonByIDAll[dl.ID] = dl
	}
	for _, da := range deletedAssigns {
		u, uOk := userByIDAll[da.UserID]
		l, lOk := lessonByIDAll[da.LessonID]
		if uOk && lOk && da.DeletedAt.Valid {
			snap.DeletedAssignments = append(snap.DeletedAssignments, DeletedAssignmentTombstoneDTO{
				LessonTitle: l.Title,
				UserEmail:   u.Email,
				UserWallet:  u.WalletAddress,
				DeletedAt:   da.DeletedAt.Time,
			})
		}
	}

	// Tombstones for moderated/deleted learner comments (90-day TTL)
	var modRatings []models.LessonRating
	database.DB.Where("comment_moderated_at IS NOT NULL AND comment_moderated_at >= ?", tombstoneCutoff).Find(&modRatings)
	for _, mr := range modRatings {
		u, uOk := userByIDAll[mr.UserID]
		l, lOk := lessonByIDAll[mr.LessonID]
		if uOk && lOk && mr.CommentModeratedAt != nil {
			snap.ModeratedComments = append(snap.ModeratedComments, ModeratedCommentTombstoneDTO{
				LessonTitle: l.Title,
				UserEmail:   u.Email,
				UserWallet:  u.WalletAddress,
				DeletedAt:   *mr.CommentModeratedAt,
			})
		}
	}

	userByID := make(map[uint]models.User)
	for _, u := range users {
		userByID[u.ID] = u
	}
	lessonByID := make(map[uint]models.Lesson)
	for _, l := range lessons {
		lessonByID[l.ID] = l
	}

	var assigns []models.LessonAssignment
	database.DB.Find(&assigns)
	for _, a := range assigns {
		u, uOk := userByID[a.UserID]
		l, lOk := lessonByID[a.LessonID]
		if uOk && lOk {
			snap.Assignments = append(snap.Assignments, SyncAssignmentDTO{
				LessonTitle: l.Title,
				UserEmail:   u.Email,
				UserWallet:  u.WalletAddress,
				CreatedAt:   a.CreatedAt,
			})
		}
	}

	var progresses []models.UserLessonProgress
	database.DB.Find(&progresses)
	for _, p := range progresses {
		u, uOk := userByID[p.UserID]
		l, lOk := lessonByID[p.LessonID]
		if !uOk || !lOk {
			continue
		}
		var bestScore, totalScore float64
		for _, q := range l.Quizzes {
			if q.Type == "lesson_post_test" {
				var attempt models.QuizAttempt
				if err := database.DB.Where("user_id = ? AND quiz_id = ?", u.ID, q.ID).Order("score desc").First(&attempt).Error; err == nil {
					bestScore = attempt.Score
					totalScore = attempt.TotalScore
				}
			}
		}
		snap.ProgressList = append(snap.ProgressList, SyncProgressDTO{
			UserEmail:      u.Email,
			UserWallet:     u.WalletAddress,
			LessonTitle:    l.Title,
			LastWatchedSec: p.LastWatchedSec,
			VideoWatched:   p.VideoWatched,
			PreTestPassed:  p.PreTestPassed,
			PostTestPassed: p.PostTestPassed,
			IsCompleted:    p.IsCompleted,
			OnChainTxHash:  p.OnChainTxHash,
			BestScore:      bestScore,
			TotalScore:     totalScore,
		})
	}

	var certs []models.Certificate
	database.DB.Find(&certs)
	for _, cert := range certs {
		u, uOk := userByID[cert.UserID]
		if !uOk {
			continue
		}
		lTitle := cert.LessonTitle
		if lTitle == "" {
			if l, ok := lessonByID[cert.LessonID]; ok {
				lTitle = l.Title
			}
		}
		snap.Certificates = append(snap.Certificates, SyncCertificateDTO{
			UserEmail:         u.Email,
			UserWallet:        u.WalletAddress,
			LessonTitle:       lTitle,
			StudyHours:        cert.StudyHours,
			LessonDurationSec: cert.LessonDurationSec,
			TokenID:           cert.TokenID,
			TransactionHash:   cert.TransactionHash,
			LessonsCount:      cert.LessonsCount,
			TotalWatchSec:     cert.TotalWatchSec,
			AverageScore:      cert.AverageScore,
			IssuedAt:          cert.IssuedAt,
		})
	}

	return snap
}

func looksMojibakeStr(s string) bool {
	return strings.Contains(s, "Ã") || strings.Contains(s, "à¸") || strings.Contains(s, "à¹")
}

func repairMojibakeStr(s string) string {
	cur := s
	for pass := 0; pass < 5 && looksMojibakeStr(cur); pass++ {
		var buf []byte
		ok := true
		for _, r := range cur {
			if r > 255 {
				ok = false
				break
			}
			buf = append(buf, byte(r))
		}
		if !ok || !utf8.Valid(buf) {
			break
		}
		next := string(buf)
		if next == cur {
			break
		}
		cur = next
	}
	return cur
}

// MergeSnapshot merges an incoming SyncSnapshot into the local SQLite database without duplicates
func MergeSnapshot(snap *SyncSnapshot) fiber.Map {
	if snap == nil {
		return fiber.Map{}
	}
	for i := range snap.Users {
		snap.Users[i].Name = repairMojibakeStr(snap.Users[i].Name)
		snap.Users[i].Ward = repairMojibakeStr(snap.Users[i].Ward)
	}
	for i := range snap.Lessons {
		snap.Lessons[i].Title = repairMojibakeStr(snap.Lessons[i].Title)
		snap.Lessons[i].Summary = repairMojibakeStr(snap.Lessons[i].Summary)
		snap.Lessons[i].Description = repairMojibakeStr(snap.Lessons[i].Description)
		snap.Lessons[i].OpenPeriod = repairMojibakeStr(snap.Lessons[i].OpenPeriod)
	}
	for i := range snap.Certificates {
		snap.Certificates[i].LessonTitle = repairMojibakeStr(snap.Certificates[i].LessonTitle)
	}
	usersAdded, usersUpdated := 0, 0
	lessonsAdded, assignsAdded := 0, 0
	progressAdded, progressUpdated := 0, 0
	certsAdded := 0

	// 1. Ensure at least one Course exists
	var defaultCourse models.Course
	if err := database.DB.First(&defaultCourse).Error; err != nil {
		defaultCourse = models.Course{
			Title:       "หลักสูตรการพยาบาล",
			Summary:     "บทเรียนออนไลน์ทางการพยาบาล",
			Description: "บทเรียนออนไลน์ทางการพยาบาล",
			CreatorID:   1,
		}
		database.DB.Create(&defaultCourse)
	}

	// 2. Merge Users (Last-Write-Wins via UpdatedAt + DeletedUsers tombstone)
	deletedUserMap := make(map[string]time.Time)
	for _, du := range snap.DeletedUsers {
		if du.Email != "" {
			k := strings.ToLower(du.Email) + "|" + du.Role
			if prev, ok := deletedUserMap[k]; !ok || du.DeletedAt.After(prev) {
				deletedUserMap[k] = du.DeletedAt
			}
		}
		if du.WalletAddress != "" {
			if prev, ok := deletedUserMap[du.WalletAddress]; !ok || du.DeletedAt.After(prev) {
				deletedUserMap[du.WalletAddress] = du.DeletedAt
			}
		}
	}

	for _, su := range snap.Users {
		uKey := strings.ToLower(su.Email) + "|" + su.Role
		if delAt, isDel := deletedUserMap[uKey]; isDel && (su.UpdatedAt.IsZero() || !su.UpdatedAt.After(delAt)) {
			continue
		}
		if delAt, isDel := deletedUserMap[su.WalletAddress]; isDel && (su.UpdatedAt.IsZero() || !su.UpdatedAt.After(delAt)) {
			continue
		}

		var existing models.User
		err := database.DB.Where("wallet_address = ?", su.WalletAddress).First(&existing).Error
		if err != nil && su.Email != "" {
			err = database.DB.Where("LOWER(email) = ? AND role = ?", strings.ToLower(su.Email), su.Role).First(&existing).Error
		}
		if err != nil {
			newU := models.User{
				WalletAddress: su.WalletAddress,
				Email:         su.Email,
				PasswordHash:  su.PasswordHash,
				Name:          su.Name,
				Ward:          su.Ward,
				Role:          su.Role,
				AvatarURL:     su.AvatarURL,
			}
			if database.DB.Create(&newU).Error == nil {
				usersAdded++
			}
		} else {
			isIncomingNewer := su.UpdatedAt.IsZero() || !existing.UpdatedAt.After(su.UpdatedAt)
			changed := false
			if existing.PasswordHash == "" && su.PasswordHash != "" {
				existing.PasswordHash = su.PasswordHash
				changed = true
			}
			if isIncomingNewer && su.Name != "" && existing.Name != su.Name {
				existing.Name = su.Name
				changed = true
			}
			if isIncomingNewer && su.Ward != "" && existing.Ward != su.Ward {
				existing.Ward = su.Ward
				changed = true
			}
			if isIncomingNewer && su.AvatarURL != "" && existing.AvatarURL != su.AvatarURL {
				existing.AvatarURL = su.AvatarURL
				changed = true
			}
			if changed {
				database.DB.Save(&existing)
				usersUpdated++
			}
		}
	}

	// Soft-delete any local users flagged in DeletedUsers tombstone
	for _, du := range snap.DeletedUsers {
		var u models.User
		var err error
		if du.Email != "" {
			err = database.DB.Where("LOWER(email) = ? AND role = ?", strings.ToLower(du.Email), du.Role).First(&u).Error
		}
		if err != nil && du.WalletAddress != "" {
			err = database.DB.Where("wallet_address = ?", du.WalletAddress).First(&u).Error
		}
		if err == nil && u.UpdatedAt.Before(du.DeletedAt) {
			database.DB.Delete(&u)
		}
	}

	// 2.5 Write synced custom cover files into ./covers/
	if len(snap.CoverFiles) > 0 {
		_ = os.MkdirAll(coversDir, 0755)
		for _, cf := range snap.CoverFiles {
			fname := filepath.Base(cf.Filename)
			if fname == "" || fname == "." || fname == ".." || cf.DataBase64 == "" {
				continue
			}
			if rawBytes, err := base64.StdEncoding.DecodeString(cf.DataBase64); err == nil && len(rawBytes) > 0 {
				_ = os.WriteFile(filepath.Join(coversDir, fname), rawBytes, 0644)
			}
		}
	}

	// 3. Build blocked/deleted lesson tombstone set (timestamp-aware so re-created lessons with newer UpdatedAt are allowed)
	permanentlyBlockedTitles := make(map[string]bool)
	for _, retired := range database.RetiredDemoLessonTitles {
		permanentlyBlockedTitles[retired] = true
	}
	deletedLessonTimeMap := make(map[string]time.Time)
	for _, dl := range snap.DeletedLessons {
		if dl.Title != "" {
			if prev, ok := deletedLessonTimeMap[dl.Title]; !ok || dl.DeletedAt.After(prev) {
				deletedLessonTimeMap[dl.Title] = dl.DeletedAt
			}
		}
	}
	for _, dt := range snap.DeletedLessonTitles {
		if dt != "" {
			if _, hasTs := deletedLessonTimeMap[dt]; !hasTs {
				permanentlyBlockedTitles[dt] = true
			}
		}
	}
	var localDeletedLessons []models.Lesson
	database.DB.Unscoped().Where("deleted_at IS NOT NULL").Find(&localDeletedLessons)
	for _, dl := range localDeletedLessons {
		if dl.Title != "" && dl.DeletedAt.Valid {
			if prev, ok := deletedLessonTimeMap[dl.Title]; !ok || dl.DeletedAt.Time.After(prev) {
				deletedLessonTimeMap[dl.Title] = dl.DeletedAt.Time
			}
		}
	}

	isLessonBlocked := func(title string, updatedAt time.Time) bool {
		if permanentlyBlockedTitles[title] {
			return true
		}
		if delAt, ok := deletedLessonTimeMap[title]; ok {
			if updatedAt.IsZero() || !updatedAt.After(delAt) {
				return true
			}
		}
		return false
	}

	// Soft-delete any locally active lesson that is permanently retired or deleted after its UpdatedAt
	var activeLocalLessons []models.Lesson
	database.DB.Find(&activeLocalLessons)
	for _, al := range activeLocalLessons {
		if isLessonBlocked(al.Title, al.UpdatedAt) {
			database.DB.Delete(&al)
		}
	}

	// Merge Lessons & Quizzes (skipping blocked/deleted lessons)
	for _, sl := range snap.Lessons {
		if isLessonBlocked(sl.Title, sl.UpdatedAt) {
			continue
		}
		var existingLesson models.Lesson
		err := database.DB.Where("title = ?", sl.Title).First(&existingLesson).Error
		if err != nil {
			passPct := sl.PassScorePct
			if passPct <= 0 {
				passPct = 70
			}
			newLesson := models.Lesson{
				CourseID:        defaultCourse.ID,
				SortOrder:       sl.SortOrder,
				Title:           sl.Title,
				Summary:         sl.Summary,
				Description:     sl.Description,
				YoutubeURL:      sl.YoutubeURL,
				ThumbnailURL:    sl.ThumbnailURL,
				DocumentURL:     sl.DocumentURL,
				DurationSec:     sl.DurationSec,
				StudyHours:      sl.StudyHours,
				OpenPeriod:      sl.OpenPeriod,
				IsHidden:        sl.IsHidden,
				PassScorePct:    passPct,
				DisablePreTest:  sl.DisablePreTest,
				DisablePostTest: sl.DisablePostTest,
			}
			if database.DB.Create(&newLesson).Error == nil {
				lessonsAdded++
				for _, q := range sl.Quizzes {
					newQuiz := models.Quiz{
						LessonID: newLesson.ID,
						Type:     q.Type,
						Title:    q.Title,
					}
					if database.DB.Create(&newQuiz).Error == nil {
						for _, qu := range q.Questions {
							newQ := models.Question{
								QuizID:       newQuiz.ID,
								QuestionText: qu.QuestionText,
							}
							if database.DB.Create(&newQ).Error == nil {
								for _, op := range qu.Options {
									newOpt := models.Option{
										QuestionID: newQ.ID,
										OptionText: op.OptionText,
										IsCorrect:  op.IsCorrect,
									}
									database.DB.Create(&newOpt)
								}
							}
						}
					}
				}
			}
		} else {
			// LWW: only overwrite local lesson edits if incoming lesson is newer (or legacy without UpdatedAt)
			if !sl.UpdatedAt.IsZero() && existingLesson.UpdatedAt.After(sl.UpdatedAt) {
				continue
			}
			oldThumb := existingLesson.ThumbnailURL
			existingLesson.Summary = sl.Summary
			existingLesson.Description = sl.Description
			existingLesson.YoutubeURL = sl.YoutubeURL
			if sl.ThumbnailURL != "" {
				existingLesson.ThumbnailURL = sl.ThumbnailURL
			}
			if sl.DocumentURL != "" {
				existingLesson.DocumentURL = sl.DocumentURL
			}
			existingLesson.DurationSec = sl.DurationSec
			existingLesson.StudyHours = sl.StudyHours
			existingLesson.OpenPeriod = sl.OpenPeriod
			existingLesson.IsHidden = sl.IsHidden
			if sl.PassScorePct > 0 {
				existingLesson.PassScorePct = sl.PassScorePct
			}
			existingLesson.DisablePreTest = sl.DisablePreTest
			existingLesson.DisablePostTest = sl.DisablePostTest
			database.DB.Save(&existingLesson)
			if oldThumb != "" && oldThumb != existingLesson.ThumbnailURL {
				RemoveCoverIfUnused(oldThumb)
			}
		}
	}

	for _, dc := range snap.DeletedCovers {
		RemoveCoverIfUnused(dc)
	}

	// Build lookup helpers after user & lesson merge
	findLearner := func(email, wallet string) (models.User, bool) {
		var u models.User
		if email != "" {
			if err := database.DB.Where("LOWER(email) = ? AND role = 'learner'", strings.ToLower(email)).First(&u).Error; err == nil {
				return u, true
			}
			if err := database.DB.Where("LOWER(email) = ?", strings.ToLower(email)).First(&u).Error; err == nil {
				return u, true
			}
		}
		if wallet != "" {
			if err := database.DB.Where("wallet_address = ?", wallet).First(&u).Error; err == nil {
				return u, true
			}
		}
		return u, false
	}

	findLesson := func(title string) (models.Lesson, bool) {
		var l models.Lesson
		if err := database.DB.Preload("Quizzes").Where("title = ?", title).First(&l).Error; err == nil {
			return l, true
		}
		return l, false
	}

	// 4. Merge Assignments + apply DeletedAssignments tombstones
	deletedAssignMap := make(map[string]time.Time)
	for _, da := range snap.DeletedAssignments {
		k := da.LessonTitle + "|" + strings.ToLower(da.UserEmail)
		if prev, ok := deletedAssignMap[k]; !ok || da.DeletedAt.After(prev) {
			deletedAssignMap[k] = da.DeletedAt
		}
	}
	for _, da := range snap.DeletedAssignments {
		u, uOk := findLearner(da.UserEmail, da.UserWallet)
		l, lOk := findLesson(da.LessonTitle)
		if uOk && lOk {
			var existingAssign models.LessonAssignment
			if err := database.DB.Where("lesson_id = ? AND user_id = ?", l.ID, u.ID).First(&existingAssign).Error; err == nil {
				if existingAssign.UpdatedAt.Before(da.DeletedAt) {
					database.DB.Delete(&existingAssign)
				}
			}
		}
	}

	for _, sa := range snap.Assignments {
		k := sa.LessonTitle + "|" + strings.ToLower(sa.UserEmail)
		if delAt, isDel := deletedAssignMap[k]; isDel && !sa.CreatedAt.After(delAt) {
			continue
		}
		u, uOk := findLearner(sa.UserEmail, sa.UserWallet)
		l, lOk := findLesson(sa.LessonTitle)
		if !uOk || !lOk {
			continue
		}
		var count int64
		database.DB.Model(&models.LessonAssignment{}).Where("lesson_id = ? AND user_id = ?", l.ID, u.ID).Count(&count)
		if count == 0 {
			if database.DB.Create(&models.LessonAssignment{
				LessonID: l.ID,
				UserID:   u.ID,
			}).Error == nil {
				assignsAdded++
			}
		}
	}

	// 4.5 Apply ModeratedComments tombstones (ensure deleted inappropriate comments stay deleted)
	for _, mc := range snap.ModeratedComments {
		u, uOk := findLearner(mc.UserEmail, mc.UserWallet)
		l, lOk := findLesson(mc.LessonTitle)
		if uOk && lOk {
			var rating models.LessonRating
			if err := database.DB.Where("lesson_id = ? AND user_id = ?", l.ID, u.ID).First(&rating).Error; err == nil {
				if rating.UpdatedAt.Before(mc.DeletedAt) && rating.Comment != "" {
					modAt := mc.DeletedAt
					database.DB.Model(&rating).Updates(map[string]interface{}{
						"comment":              "",
						"is_reported":          false,
						"report_reason":        "",
						"reported_by":          "",
						"reported_at":          nil,
						"comment_moderated_at": &modAt,
					})
				}
			}
		}
	}

	// 5. Merge Learning Progress & Quiz Attempts
	for _, sp := range snap.ProgressList {
		u, uOk := findLearner(sp.UserEmail, sp.UserWallet)
		l, lOk := findLesson(sp.LessonTitle)
		if !uOk || !lOk {
			continue
		}
		var prog models.UserLessonProgress
		err := database.DB.Where("user_id = ? AND lesson_id = ?", u.ID, l.ID).First(&prog).Error
		if err != nil {
			prog = models.UserLessonProgress{
				UserID:         u.ID,
				LessonID:       l.ID,
				LastWatchedSec: sp.LastWatchedSec,
				VideoWatched:   sp.VideoWatched,
				PreTestPassed:  sp.PreTestPassed,
				PostTestPassed: sp.PostTestPassed,
				IsCompleted:    sp.IsCompleted,
				OnChainTxHash:  sp.OnChainTxHash,
			}
			if database.DB.Create(&prog).Error == nil {
				progressAdded++
			}
		} else {
			changed := false
			if sp.LastWatchedSec > prog.LastWatchedSec {
				prog.LastWatchedSec = sp.LastWatchedSec
				changed = true
			}
			if sp.VideoWatched && !prog.VideoWatched {
				prog.VideoWatched = true
				changed = true
			}
			if sp.PreTestPassed && !prog.PreTestPassed {
				prog.PreTestPassed = true
				changed = true
			}
			if sp.PostTestPassed && !prog.PostTestPassed {
				prog.PostTestPassed = true
				changed = true
			}
			if sp.IsCompleted && !prog.IsCompleted {
				prog.IsCompleted = true
				changed = true
			}
			if sp.OnChainTxHash != "" && prog.OnChainTxHash == "" {
				prog.OnChainTxHash = sp.OnChainTxHash
				changed = true
			}
			if changed {
				database.DB.Save(&prog)
				progressUpdated++
			}
		}

		if sp.BestScore > 0 || sp.PostTestPassed {
			for _, q := range l.Quizzes {
				if q.Type == "lesson_post_test" {
					var attCount int64
					database.DB.Model(&models.QuizAttempt{}).Where("user_id = ? AND quiz_id = ?", u.ID, q.ID).Count(&attCount)
					if attCount == 0 {
						tot := sp.TotalScore
						if tot <= 0 {
							tot = 1
						}
						database.DB.Create(&models.QuizAttempt{
							UserID:        u.ID,
							QuizID:        q.ID,
							Score:         sp.BestScore,
							TotalScore:    tot,
							IsPassed:      sp.PostTestPassed,
							OnChainTxHash: sp.OnChainTxHash,
						})
					}
				}
			}
		}
	}

	// 6. Merge Certificates
	for _, sc := range snap.Certificates {
		u, uOk := findLearner(sc.UserEmail, sc.UserWallet)
		if !uOk {
			continue
		}
		l, _ := findLesson(sc.LessonTitle)
		var existingCert models.Certificate
		err := database.DB.Where("token_id = ?", sc.TokenID).First(&existingCert).Error
		if err != nil && l.ID > 0 {
			err = database.DB.Where("user_id = ? AND lesson_id = ?", u.ID, l.ID).First(&existingCert).Error
		}
		if err != nil {
			newCert := models.Certificate{
				UserID:            u.ID,
				CourseID:          defaultCourse.ID,
				LessonID:          l.ID,
				LessonTitle:       sc.LessonTitle,
				StudyHours:        sc.StudyHours,
				LessonDurationSec: sc.LessonDurationSec,
				TokenID:           sc.TokenID,
				TransactionHash:   sc.TransactionHash,
				LessonsCount:      sc.LessonsCount,
				TotalWatchSec:     sc.TotalWatchSec,
				AverageScore:      sc.AverageScore,
				IssuedAt:          sc.IssuedAt,
			}
			if database.DB.Create(&newCert).Error == nil {
				certsAdded++
			}
		}
	}

	return fiber.Map{
		"users_added":      usersAdded,
		"users_updated":    usersUpdated,
		"lessons_added":    lessonsAdded,
		"assigns_added":    assignsAdded,
		"progress_added":   progressAdded,
		"progress_updated": progressUpdated,
		"certs_added":      certsAdded,
	}
}

// ExportSnapshot returns the full JSON snapshot of this machine
func (h *SyncHandler) ExportSnapshot(c *fiber.Ctx) error {
	snap := BuildSnapshot()
	raw, err := json.Marshal(snap)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Send(raw)
}

// ImportSnapshot merges an uploaded or posted JSON snapshot into this machine
func (h *SyncHandler) ImportSnapshot(c *fiber.Ctx) error {
	var wrapper struct {
		Snapshot *SyncSnapshot `json:"snapshot"`
	}
	var snap SyncSnapshot

	body := c.Body()
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Snapshot != nil && (len(wrapper.Snapshot.Users) > 0 || len(wrapper.Snapshot.Lessons) > 0) {
		snap = *wrapper.Snapshot
	} else if err := json.Unmarshal(body, &snap); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "รูปแบบข้อมูลสำหรับซิงค์ไม่ถูกต้อง",
		})
	}

	stats := MergeSnapshot(&snap)
	return c.JSON(fiber.Map{
		"success": true,
		"message": "นำเข้าและผสานข้อมูลสำเร็จเรียบร้อยแล้ว",
		"stats":   stats,
	})
}

// PeerSync performs a two-way sync (Pull + Push) with another machine running Learn Chain Nurse.exe
func (h *SyncHandler) PeerSync(c *fiber.Ctx) error {
	var req struct {
		PeerURL string `json:"peer_url"`
	}
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.PeerURL) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "กรุณาระบุ URL ของเครื่องปลายทาง หรือ Google Apps Script URL",
		})
	}

	// If the URL is a Google Apps Script URL or gas-hub URL, use ExecuteCloudAndPeerAutoSync
	if strings.Contains(req.PeerURL, "script.google.com") || strings.Contains(req.PeerURL, "gas-hub") {
		h.mu.Lock()
		h.cfg.GasURL = strings.TrimSpace(req.PeerURL)
		h.saveConfigToFile()
		h.mu.Unlock()
		resMap, err := h.ExecuteCloudAndPeerAutoSync()
		if err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
				"success": false,
				"error":   err.Error(),
			})
		}
		return c.JSON(resMap)
	}

	peerBase := strings.TrimRight(strings.TrimSpace(req.PeerURL), "/")
	if !strings.HasPrefix(peerBase, "http://") && !strings.HasPrefix(peerBase, "https://") {
		peerBase = "http://" + peerBase
	}
	if !strings.Contains(strings.TrimPrefix(strings.TrimPrefix(peerBase, "http://"), "https://"), ":") {
		peerBase = peerBase + ":80"
	}

	// Use gas-hub endpoint on peer so active node registration & timeout tracking also happens automatically!
	h.mu.Lock()
	if h.cfg.GasURL == "" {
		h.cfg.GasURL = peerBase
		h.saveConfigToFile()
	}
	h.mu.Unlock()

	nodeID, hostname, primaryLanURL, _ := getMachineIdentity()
	localSnap := BuildSnapshot()
	pubIP, _, _ := getPublicIPAndWebStatus()
	hubPayload, _ := json.Marshal(fiber.Map{
		"action":      "auto_sync",
		"node_id":     nodeID,
		"hostname":    hostname,
		"lan_url":     primaryLanURL,
		"public_ip":   pubIP,
		"app_version": version.Version,
		"timeout_sec": h.cfg.TimeoutSec,
		"snapshot":    localSnap,
	})

	client := newSyncHTTPClient(20 * time.Second)
	resp, err := client.Post(peerBase+"/api/sync/gas-hub", "application/json", bytes.NewReader(hubPayload))
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   fmt.Sprintf("ไม่สามารถเชื่อมต่อกับเครื่องปลายทาง (%s) ได้: %v", peerBase, err),
		})
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   "อ่านข้อมูลจากเครื่องปลายทางไม่สำเร็จ",
		})
	}

	var hubResp struct {
		Success     bool             `json:"success"`
		OnlineCount int              `json:"online_count"`
		ActiveNodes []ActiveNodeInfo `json:"active_nodes"`
		Snapshot    SyncSnapshot     `json:"snapshot"`
	}
	if err := json.Unmarshal(respBytes, &hubResp); err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"error":   "ข้อมูลตอบกลับจากเครื่องปลายทางไม่ถูกต้อง",
		})
	}

	pulledStats := MergeSnapshot(&hubResp.Snapshot)

	h.mu.Lock()
	for _, n := range hubResp.ActiveNodes {
		n.IsSelf = (n.NodeID == nodeID)
		h.activeNodes[n.NodeID] = n
	}
	h.touchSelfNode()
	activeList := h.pruneExpiredNodesLocked()
	h.lastSyncAt = time.Now().Format("15:04:05")
	h.lastSyncStatus = fmt.Sprintf("✅ ซิงค์สำเร็จ (ออนไลน์ %d เครื่อง)", len(activeList))
	h.mu.Unlock()

	return c.JSON(fiber.Map{
		"success":      true,
		"peer_url":     peerBase,
		"online_count": len(activeList),
		"active_nodes": activeList,
		"message":      fmt.Sprintf("✅ ซิงค์ข้อมูล 2 ทางกับ %s สำเร็จ! (ออนไลน์ %d เครื่อง)", peerBase, len(activeList)),
		"pulled_stats": pulledStats,
	})
}

// TrayStatusInfo holds live IP, UserNode URL, and connected MasterNodes for the System Tray popup
type TrayStatusInfo struct {
	LocalURL    string
	UserNodeIP  string
	UserNodeURL string
	MasterNodes []ActiveNodeInfo
}

// GetTrayStatus returns thread-safe node & LAN IP status for the Windows System Tray hover card
func (h *SyncHandler) GetTrayStatus(port string) TrayStatusInfo {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.touchSelfNode()
	nodes := h.pruneExpiredNodesLocked()

	ips := GetLocalIPs()
	primaryIP := "127.0.0.1"
	if len(ips) > 0 {
		primaryIP = ips[0]
	}
	userNodeURL := FormatHTTPURL(primaryIP, port)
	localURL := FormatHTTPURL("localhost", port)

	return TrayStatusInfo{
		LocalURL:    localURL,
		UserNodeIP:  primaryIP,
		UserNodeURL: userNodeURL,
		MasterNodes: nodes,
	}
}

// FetchCoverFromPeers tries to download a missing cover image from any reachable active or known peer
// and caches it locally in ./covers/.
func (h *SyncHandler) FetchCoverFromPeers(fname string) ([]byte, bool) {
	fname = filepath.Base(fname)
	if fname == "" || fname == "." || fname == ".." {
		return nil, false
	}

	selfID, _, selfLanURL, _ := getMachineIdentity()
	h.mu.Lock()
	var candidateURLs []string
	seen := make(map[string]bool)
	addPeerURLs := func(n ActiveNodeInfo) {
		if n.NodeID == selfID {
			return
		}
		for _, u := range collectNodeURLs(n) {
			if u != "" && u != selfLanURL && !seen[u] {
				seen[u] = true
				candidateURLs = append(candidateURLs, u)
			}
		}
	}
	for _, n := range h.activeNodes {
		addPeerURLs(n)
	}
	for _, n := range h.cfg.KnownPeers {
		addPeerURLs(n)
	}
	h.mu.Unlock()

	if len(candidateURLs) > 0 {
		client := newSyncHTTPClient(3 * time.Second)
		for _, base := range candidateURLs {
			resp, err := client.Get(strings.TrimRight(base, "/") + "/covers/" + fname + "?no_peer_fetch=1")
			if err != nil {
				continue
			}
			ct := strings.ToLower(resp.Header.Get("Content-Type"))
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 15*1024*1024))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK && readErr == nil && len(data) > 0 && strings.HasPrefix(ct, "image/") {
				_ = os.MkdirAll(coversDir, 0755)
				_ = os.WriteFile(filepath.Join(coversDir, fname), data, 0644)
				return data, true
			}
		}
	}

	return h.FetchCoverFromGAS(fname)
}

func (h *SyncHandler) getGASEndpoint() string {
	h.mu.Lock()
	targetURL := strings.TrimSpace(h.cfg.GasURL)
	h.mu.Unlock()
	if strings.Contains(targetURL, "script.google.com") {
		return targetURL
	}
	return DefaultGASSignalingURL
}

// UploadCoverToGAS uploads a single cover image to GAS (which stores it in Google Drive PicSync/covers).
func (h *SyncHandler) UploadCoverToGAS(filename string, rawBytes []byte) {
	if h == nil || len(rawBytes) == 0 {
		return
	}
	fname := filepath.Base(filename)
	if fname == "" || fname == "." || fname == ".." {
		return
	}
	gasEndpoint := h.getGASEndpoint()
	if gasEndpoint == "" {
		return
	}
	payload, _ := json.Marshal(fiber.Map{
		"action":      "upload_cover",
		"filename":    fname,
		"data_base64": base64.StdEncoding.EncodeToString(rawBytes),
	})
	client := newSyncHTTPClient(25 * time.Second)
	resp, err := client.Post(gasEndpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("⚠️ [Cover->GAS] อัปโหลด %s ไม่สำเร็จ: %v", fname, err)
		return
	}
	resp.Body.Close()
}

// FetchCoverFromGAS fetches a single missing cover image on-demand from GAS (Google Drive PicSync/covers) and caches it in ./covers/.
func (h *SyncHandler) FetchCoverFromGAS(fname string) ([]byte, bool) {
	fname = filepath.Base(fname)
	if fname == "" || fname == "." || fname == ".." {
		return nil, false
	}
	gasEndpoint := h.getGASEndpoint()
	if gasEndpoint == "" {
		return nil, false
	}
	sep := "?"
	if strings.Contains(gasEndpoint, "?") {
		sep = "&"
	}
	reqURL := gasEndpoint + sep + "action=get_cover&filename=" + url.QueryEscape(fname)
	client := newSyncHTTPClient(15 * time.Second)
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 15*1024*1024))
	if err != nil {
		return nil, false
	}
	var parsed struct {
		Success    bool   `json:"success"`
		DataBase64 string `json:"data_base64"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil || !parsed.Success || parsed.DataBase64 == "" {
		return nil, false
	}
	rawBytes, err := base64.StdEncoding.DecodeString(parsed.DataBase64)
	if err != nil || len(rawBytes) == 0 {
		return nil, false
	}
	_ = os.MkdirAll(coversDir, 0755)
	_ = os.WriteFile(filepath.Join(coversDir, fname), rawBytes, 0644)
	return rawBytes, true
}

// CleanPullFromHub pulls the latest snapshot from Master (LAN) or GAS without pushing stale local data first.
// Ideal for machines returning from long-term offline periods (> 30 days).
func (h *SyncHandler) CleanPullFromHub(c *fiber.Ctx) error {
	h.mu.Lock()
	preferredIP := strings.TrimSpace(h.cfg.PreferredMasterIP)
	masterID := h.electedMasterID
	var masterCand *ActiveNodeInfo
	if masterID != "" {
		if n, ok := h.activeNodes[masterID]; ok && !n.IsSelf {
			copyN := n
			masterCand = &copyN
		}
	}
	h.mu.Unlock()

	var pulled *SyncSnapshot
	usedSource := ""

	// 1. Try elected Master on LAN first
	if masterCand != nil {
		bestURL, _, ok := h.selectBestNodeURL(collectNodeURLs(*masterCand), preferredIP)
		if ok {
			client := newSyncHTTPClient(15 * time.Second)
			if resp, err := client.Get(strings.TrimRight(bestURL, "/") + "/api/sync/export"); err == nil {
				bodyBytes, rErr := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
				resp.Body.Close()
				if rErr == nil && resp.StatusCode == http.StatusOK {
					var s SyncSnapshot
					if json.Unmarshal(bodyBytes, &s) == nil {
						pulled = &s
						usedSource = "LAN Master (" + bestURL + ")"
					}
				}
			}
		}
	}

	// 2. Fallback to GAS GET (read-only pull of cloud snapshot)
	if pulled == nil {
		gasURL := h.getGASEndpoint()
		if gasURL != "" {
			client := newSyncHTTPClient(45 * time.Second)
			if resp, err := client.Get(gasURL); err == nil {
				bodyBytes, rErr := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
				resp.Body.Close()
				if rErr == nil && resp.StatusCode == http.StatusOK {
					var hub hubSyncResponse
					if json.Unmarshal(bodyBytes, &hub) == nil && hub.Success && hub.Snapshot != nil {
						pulled = hub.Snapshot
						usedSource = "Google Apps Script (Cloud)"
					}
				}
			}
		}
	}

	if pulled == nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถดึงข้อมูลล่าสุดจาก Master หรือ GAS ได้ในขณะนี้",
		})
	}

	stats := MergeSnapshot(pulled)

	h.mu.Lock()
	h.syncSuccessCount++
	h.cfg.LastSuccessfulSyncMs = time.Now().UnixMilli()
	h.saveConfigToFile()
	h.lastSyncAt = time.Now().Format("15:04:05")
	h.lastSyncStatus = "✅ Clean Pull สำเร็จจาก " + usedSource
	h.mu.Unlock()

	return c.JSON(fiber.Map{
		"success": true,
		"source":  usedSource,
		"message": "✅ ดึงข้อมูลล่าสุดจาก " + usedSource + " เรียบร้อยแล้ว",
		"stats":   stats,
	})
}
