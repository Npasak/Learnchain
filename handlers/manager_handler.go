package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"elearning-sbt/database"
	"elearning-sbt/middleware"
	"elearning-sbt/models"
	"elearning-sbt/services"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gofiber/fiber/v2"
)

type CreatorCandidateDTO struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	WalletAddress string `json:"wallet_address"`
	IsManager     bool   `json:"is_manager"`
}

type ManagerHandler struct {
	emailService *services.EmailService
}

func NewManagerHandler(es *services.EmailService) *ManagerHandler {
	return &ManagerHandler{emailService: es}
}

type LessonManagerStatsDTO struct {
	LessonID          uint                         `json:"lesson_id"`
	SortOrder         int                          `json:"sort_order"`
	LessonTitle       string                       `json:"lesson_title"`
	ThumbnailURL      string                       `json:"thumbnail_url"`
	OpenPeriod        string                       `json:"open_period"`
	IsHidden          bool                         `json:"is_hidden"`
	DurationSec       float64                      `json:"duration_sec"`
	CourseTitle       string                       `json:"course_title"`
	CreatorName       string                       `json:"creator_name"`
	AssignedUsers     []models.User                `json:"assigned_users"`
	LearnerStats      []LearnerProgressOverviewDTO `json:"learner_stats"`
	TotalAssigned     int                          `json:"total_assigned"`
	TotalLearners     int                          `json:"total_learners"`
	TotalCompleted    int                          `json:"total_completed"`
	TotalWatchSec     float64                      `json:"total_watch_sec"`
	AvgWatchSec       float64                      `json:"avg_watch_sec"`
	PreTestPassCount  int                          `json:"pre_test_pass_count"`
	PreTestPassPct    float64                      `json:"pre_test_pass_pct"`
	PostTestPassCount int                          `json:"post_test_pass_count"`
	PostTestPassPct   float64                      `json:"post_test_pass_pct"`
	CompletionPct     float64                      `json:"completion_pct"`
	AvgQuizScorePct   float64                      `json:"avg_quiz_score_pct"`
	AverageRating     float64                      `json:"average_rating"`
	TotalRatings      int64                        `json:"total_ratings"`
	StarCounts        map[int]int                  `json:"star_counts"`
}

type LearnerProgressOverviewDTO struct {
	UserID         uint    `json:"user_id"`
	UserName       string  `json:"user_name"`
	Email          string  `json:"email"`
	Ward           string  `json:"ward"`
	WalletAddress  string  `json:"wallet_address"`
	PreTestPassed  bool    `json:"pre_test_passed"`
	VideoWatched   bool    `json:"video_watched"`
	LastWatchedSec float64 `json:"last_watched_sec"`
	PostTestPassed bool    `json:"post_test_passed"`
	IsCompleted    bool    `json:"is_completed"`
	IsAssigned     bool    `json:"is_assigned"`
	AvgScorePct    float64 `json:"avg_score_pct"`
}

// GetDashboard returns lessons, assignments, learner lists, and detailed progress stats
func (h *ManagerHandler) GetDashboard(c *fiber.Ctx) error {
	var lessons []models.Lesson
	database.DB.Order("sort_order asc, id asc").Find(&lessons)

	var allLearners []models.User
	database.DB.Where("role = ? OR role = ''", "learner").Order("name asc").Find(&allLearners)

	var assignments []models.LessonAssignment
	database.DB.Preload("User").Find(&assignments)

	assignmentMap := make(map[uint][]models.User)
	assignmentUserSet := make(map[string]bool) // "lessonID-userID"
	for _, a := range assignments {
		assignmentMap[a.LessonID] = append(assignmentMap[a.LessonID], a.User)
		assignmentUserSet[fmt.Sprintf("%d-%d", a.LessonID, a.UserID)] = true
	}

	var allProgress []models.UserLessonProgress
	database.DB.Find(&allProgress)
	progressMap := make(map[string]models.UserLessonProgress)
	for _, p := range allProgress {
		progressMap[fmt.Sprintf("%d-%d", p.LessonID, p.UserID)] = p
	}

	userMap := make(map[uint]models.User)
	for _, u := range allLearners {
		userMap[u.ID] = u
	}

	resultLessons := make([]LessonManagerStatsDTO, 0, len(lessons))

	for _, l := range lessons {
		if ComputeBlockLifecycle(l.CreatedAt).IsExpired {
			continue
		}
		var course models.Course
		database.DB.Preload("Creator").First(&course, l.CourseID)

		assigned := assignmentMap[l.ID]
		if assigned == nil {
			assigned = []models.User{}
		}

		learnerStats := make([]LearnerProgressOverviewDTO, 0)
		completedCount := 0
		prePassCount := 0
		postPassCount := 0
		totalWatchSec := 0.0

		// Calculate average quiz score for this lesson and per user
		var attempts []models.QuizAttempt
		database.DB.Table("quiz_attempts").
			Joins("JOIN quizzes ON quizzes.id = quiz_attempts.quiz_id").
			Where("quizzes.lesson_id = ?", l.ID).
			Find(&attempts)

		userScoreSum := make(map[uint]float64)
		userScoreCnt := make(map[uint]int)
		avgQuizScorePct := 0.0
		if len(attempts) > 0 {
			sumPct := 0.0
			for _, a := range attempts {
				if a.TotalScore > 0 {
					pct := (a.Score / a.TotalScore) * 100.0
					sumPct += pct
					userScoreSum[a.UserID] += pct
					userScoreCnt[a.UserID]++
				}
			}
			avgQuizScorePct = sumPct / float64(len(attempts))
		}

		// Iterate over all known learners to show their stats in this lesson
		for _, u := range allLearners {
			pKey := fmt.Sprintf("%d-%d", l.ID, u.ID)
			prog, hasProg := progressMap[pKey]
			isAssigned := assignmentUserSet[pKey]

			// Include if assigned or has interacted
			if isAssigned || (hasProg && (prog.VideoWatched || prog.PreTestPassed || prog.PostTestPassed || prog.LastWatchedSec > 0)) {
				if prog.IsCompleted {
					completedCount++
				}
				if prog.PreTestPassed {
					prePassCount++
				}
				if prog.PostTestPassed {
					postPassCount++
				}
				totalWatchSec += prog.LastWatchedSec

				uWard := strings.TrimSpace(u.Ward)
				if uWard == "" {
					uWard = "สว.11"
				}
				uAvgScore := 0.0
				if userScoreCnt[u.ID] > 0 {
					uAvgScore = userScoreSum[u.ID] / float64(userScoreCnt[u.ID])
				}

				learnerStats = append(learnerStats, LearnerProgressOverviewDTO{
					UserID:         u.ID,
					UserName:       u.Name,
					Email:          u.Email,
					Ward:           uWard,
					WalletAddress:  u.WalletAddress,
					PreTestPassed:  prog.PreTestPassed,
					VideoWatched:   prog.VideoWatched,
					LastWatchedSec: prog.LastWatchedSec,
					PostTestPassed: prog.PostTestPassed,
					IsCompleted:    prog.IsCompleted,
					IsAssigned:     isAssigned,
					AvgScorePct:    uAvgScore,
				})
			}
		}

		totalLearners := len(learnerStats)
		avgWatchSec := 0.0
		prePassPct := 0.0
		postPassPct := 0.0
		completionPct := 0.0

		if totalLearners > 0 {
			avgWatchSec = totalWatchSec / float64(totalLearners)
			prePassPct = (float64(prePassCount) / float64(totalLearners)) * 100.0
			postPassPct = (float64(postPassCount) / float64(totalLearners)) * 100.0
			completionPct = (float64(completedCount) / float64(totalLearners)) * 100.0
		}

		creatorName := "ผู้สร้าง"
		if course.Creator.Name != "" {
			creatorName = course.Creator.Name
		}

		ratingStats := ComputeLessonRatingStats(l.ID, 0)
		resultLessons = append(resultLessons, LessonManagerStatsDTO{
			LessonID:          l.ID,
			SortOrder:         l.SortOrder,
			LessonTitle:       l.Title,
			ThumbnailURL:      l.ThumbnailURL,
			OpenPeriod:        l.OpenPeriod,
			IsHidden:          l.IsHidden,
			DurationSec:       l.DurationSec,
			CourseTitle:       course.Title,
			CreatorName:       creatorName,
			AssignedUsers:     assigned,
			LearnerStats:      learnerStats,
			TotalAssigned:     len(assigned),
			TotalLearners:     totalLearners,
			TotalCompleted:    completedCount,
			TotalWatchSec:     totalWatchSec,
			AvgWatchSec:       avgWatchSec,
			PreTestPassCount:  prePassCount,
			PreTestPassPct:    prePassPct,
			PostTestPassCount: postPassCount,
			PostTestPassPct:   postPassPct,
			CompletionPct:     completionPct,
			AvgQuizScorePct:   avgQuizScorePct,
			AverageRating:     ratingStats.AverageRating,
			TotalRatings:      ratingStats.TotalRatings,
			StarCounts:        ratingStats.StarCounts,
		})
	}

	// Fetch all Creators and Managers for the "เพิ่มผู้จัดการจากผู้สร้าง" menu
	var allCreators []models.User
	database.DB.Where("role = ?", "creator").Order("name asc").Find(&allCreators)

	var allManagers []models.User
	database.DB.Where("role = ?", "manager").Order("id asc").Find(&allManagers)

	mgrEmailSet := make(map[string]bool)
	for _, m := range allManagers {
		if m.Email != "" {
			mgrEmailSet[strings.ToLower(strings.TrimSpace(m.Email))] = true
		}
	}

	creatorCandidates := make([]CreatorCandidateDTO, 0, len(allCreators))
	for _, cr := range allCreators {
		isMgr := false
		if cr.Email != "" {
			isMgr = mgrEmailSet[strings.ToLower(strings.TrimSpace(cr.Email))]
		}
		creatorCandidates = append(creatorCandidates, CreatorCandidateDTO{
			ID:            cr.ID,
			Name:          cr.Name,
			Email:         cr.Email,
			WalletAddress: cr.WalletAddress,
			IsManager:     isMgr,
		})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"lessons":  resultLessons,
		"learners": allLearners,
		"creators": creatorCandidates,
		"managers": allManagers,
	})
}

type AddManagerFromCreatorRequest struct {
	CreatorID uint `json:"creator_id"`
}

// AddManagerFromCreator promotes a Creator user ("ผู้สร้างบทเรียน") to also have Manager ("ผู้จัดการ") privileges
func (h *ManagerHandler) AddManagerFromCreator(c *fiber.Ctx) error {
	var req AddManagerFromCreatorRequest
	if err := c.BodyParser(&req); err != nil || req.CreatorID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "กรุณาเลือกผู้สร้างบทเรียนที่ต้องการเพิ่มเป็นผู้จัดการ",
		})
	}

	var creator models.User
	if err := database.DB.Where("id = ? AND role = ?", req.CreatorID, "creator").First(&creator).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบบัญชีผู้สร้างบทเรียนที่เลือก",
		})
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(creator.Email))
	if cleanEmail == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ผู้สร้างบทเรียนท่านนี้ไม่มีข้อมูลอีเมลในระบบ",
		})
	}

	var existingMgr models.User
	if err := database.DB.Where("LOWER(email) = ? AND role = ?", cleanEmail, "manager").First(&existingMgr).Error; err == nil {
		return c.JSON(fiber.Map{
			"success": true,
			"message": fmt.Sprintf("ผู้สร้าง '%s (%s)' มีสิทธิ์เป็นผู้จัดการอยู่แล้ว", creator.Name, creator.Email),
			"manager": existingMgr,
		})
	}

	privKey, err := crypto.GenerateKey()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถสร้างกระเป๋าสำหรับบัญชีผู้จัดการได้",
		})
	}
	walletAddr := crypto.PubkeyToAddress(privKey.PublicKey).Hex()

	newManager := models.User{
		WalletAddress: walletAddr,
		Email:         creator.Email,
		Name:          creator.Name,
		Role:          "manager",
		AvatarURL:     creator.AvatarURL,
	}

	if err := database.DB.Create(&newManager).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถเพิ่มผู้จัดการได้",
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("แต่งตั้ง '%s (%s)' จากผู้สร้างบทเรียนเป็นผู้จัดการเรียบร้อยแล้ว!", creator.Name, creator.Email),
		"manager": newManager,
	})
}

// RemoveManager revokes Manager privileges from a user (keeps at least 1 manager in the system)
func (h *ManagerHandler) RemoveManager(c *fiber.Ctx) error {
	mgrIDStr := c.Params("id")
	mgrID, err := strconv.ParseUint(mgrIDStr, 10, 32)
	if err != nil || mgrID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "รหัสผู้จัดการไม่ถูกต้อง",
		})
	}

	var mgrCount int64
	database.DB.Model(&models.User{}).Where("role = ?", "manager").Count(&mgrCount)
	if mgrCount <= 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถถอนสิทธิ์ผู้จัดการคนสุดท้ายของระบบได้ (ต้องมีผู้จัดการอย่างน้อย 1 คน)",
		})
	}

	if err := database.DB.Where("id = ? AND role = ?", mgrID, "manager").Delete(&models.User{}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถถอนสิทธิ์ผู้จัดการได้",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ถอนสิทธิ์ผู้จัดการเรียบร้อยแล้ว",
	})
}

// AssignLearner assigns a learner (or an entire ward of learners) to a specific lesson
func (h *ManagerHandler) AssignLearner(c *fiber.Ctx) error {
	var req models.AssignLearnerRequest
	if err := c.BodyParser(&req); err != nil || req.LessonID == 0 || (req.UserID == 0 && strings.TrimSpace(req.Ward) == "") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "กรุณาระบุบทเรียนและผู้เรียนหรือหน่วยงานให้ถูกต้อง",
		})
	}

	// Bulk assign by Ward if req.Ward is provided
	if strings.TrimSpace(req.Ward) != "" {
		ward := strings.TrimSpace(req.Ward)
		var wardLearners []models.User
		database.DB.Where("(role = ? OR role = '') AND ward = ?", "learner", ward).Find(&wardLearners)
		if len(wardLearners) == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("ยังไม่มีผู้เรียนที่สังกัดหน่วยงาน '%s' ในระบบ", ward),
			})
		}
		addedCount := 0
		for _, u := range wardLearners {
			var existing models.LessonAssignment
			if err := database.DB.Where("lesson_id = ? AND user_id = ?", req.LessonID, u.ID).First(&existing).Error; err != nil {
				database.DB.Create(&models.LessonAssignment{
					LessonID: req.LessonID,
					UserID:   u.ID,
				})
				addedCount++
			}
		}
		return c.JSON(fiber.Map{
			"success": true,
			"message": fmt.Sprintf("มอบหมายบทเรียนให้บุคลากรหน่วยงาน '%s' จำนวน %d คน (เพิ่มใหม่ %d คน) เรียบร้อยแล้ว", ward, len(wardLearners), addedCount),
		})
	}

	var existing models.LessonAssignment
	if err := database.DB.Where("lesson_id = ? AND user_id = ?", req.LessonID, req.UserID).First(&existing).Error; err == nil {
		return c.JSON(fiber.Map{
			"success": true,
			"message": "ผู้เรียนท่านนี้ได้รับมอบหมายบทเรียนนี้อยู่แล้ว",
		})
	}

	assignment := models.LessonAssignment{
		LessonID: req.LessonID,
		UserID:   req.UserID,
	}
	if err := database.DB.Create(&assignment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถมอบหมายบทเรียนได้",
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":    true,
		"message":    "มอบหมายบทเรียนให้ผู้เรียนเรียบร้อยแล้ว",
		"assignment": assignment,
	})
}

// UnassignLearner removes assignment
func (h *ManagerHandler) UnassignLearner(c *fiber.Ctx) error {
	lessonIDStr := c.Query("lesson_id")
	userIDStr := c.Query("user_id")
	lessonID, _ := strconv.ParseUint(lessonIDStr, 10, 32)
	userID, _ := strconv.ParseUint(userIDStr, 10, 32)

	if lessonID == 0 || userID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid parameters",
		})
	}

	database.DB.Where("lesson_id = ? AND user_id = ?", lessonID, userID).Delete(&models.LessonAssignment{})

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ถอนการมอบหมายบทเรียนเรียบร้อยแล้ว",
	})
}

// SendReportEmail formats and emails manager report to manager's email
func (h *ManagerHandler) SendReportEmail(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)
	var manager models.User
	if err := database.DB.First(&manager, userID).Error; err != nil || manager.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบที่อยู่อีเมลของผู้จัดการสำหรับส่งรายงาน",
		})
	}

	// Build report HTML
	var lessons []models.Lesson
	database.DB.Order("id asc").Find(&lessons)

	var assignments []models.LessonAssignment
	database.DB.Preload("User").Find(&assignments)

	reportRows := ""
	for _, l := range lessons {
		var progressList []models.UserLessonProgress
		database.DB.Where("lesson_id = ?", l.ID).Find(&progressList)

		totalLearners := len(progressList)
		completed := 0
		totalWatch := 0.0
		for _, p := range progressList {
			if p.IsCompleted || p.PostTestPassed {
				completed++
			}
			totalWatch += p.LastWatchedSec
		}

		avgWatch := 0.0
		passPct := 0.0
		if totalLearners > 0 {
			avgWatch = totalWatch / float64(totalLearners)
			passPct = (float64(completed) / float64(totalLearners)) * 100.0
		}

		reportRows += fmt.Sprintf(`
			<tr style="border-bottom:1px solid #e2e8f0;">
				<td style="padding:10px;">#%d %s</td>
				<td style="padding:10px; text-align:center;">%d คน</td>
				<td style="padding:10px; text-align:center;">%.1f นาที (%.0fs)</td>
				<td style="padding:10px; text-align:center;">%.1f นาที (%.0fs)</td>
				<td style="padding:10px; text-align:center; color:#10b981; font-weight:bold;">%d คน (%.1f%%)</td>
			</tr>
		`, l.ID, l.Title, totalLearners, totalWatch/60.0, totalWatch, avgWatch/60.0, avgWatch, completed, passPct)
	}

	html := fmt.Sprintf(`
		<div style="font-family:'Prompt',sans-serif; max-width:750px; margin:0 auto; padding:20px; border:1px solid #e2e8f0; border-radius:12px;">
			<h2 style="color:#4f46e5; margin-bottom:4px;">📊 รายงานสรุปผลสถิติภาพรวมรายบทเรียน (Manager Report)</h2>
			<p style="color:#64748b; font-size:14px; margin-top:0;">ออกรายงานเมื่อ: %s | ผู้จัดการ: %s (%s)</p>
			
			<table style="width:100%%; border-collapse:collapse; margin-top:16px; font-size:13px;">
				<thead>
					<tr style="background:#f1f5f9; text-align:left;">
						<th style="padding:10px;">บทเรียน</th>
						<th style="padding:10px; text-align:center;">ผู้เข้าเรียน</th>
						<th style="padding:10px; text-align:center;">เวลาเรียนรวม</th>
						<th style="padding:10px; text-align:center;">เวลาเรียนเฉลี่ย/คน</th>
						<th style="padding:10px; text-align:center;">สอบผ่าน (ร้อยละ)</th>
					</tr>
				</thead>
				<tbody>
					%s
				</tbody>
			</table>
			<p style="margin-top:24px; font-size:12px; color:#94a3b8; text-align:center;">LearnChain Architecture • Blockchain-based Educational Ledger</p>
		</div>
	`, time.Now().Format("02/01/2006 15:04"), manager.Name, manager.Email, reportRows)

	go func() {
		_ = h.emailService.SendReportEmail(manager.Email, "📊 รายงานสรุปสถิติบทเรียนและการเรียนรู้ - LearnChain", html)
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("ส่งรายงานสถิติไปยังอีเมล %s เรียบร้อยแล้ว", manager.Email),
	})
}

// requireManagerOrSecretAdmin checks whether the caller is a Manager or the Secret Super-Admin
func requireManagerOrSecretAdmin(c *fiber.Ctx) (models.User, bool) {
	userID := middleware.ResolveUserID(c)
	if userID == SecretAdminUserID {
		return BuildSecretAdminUser("manager"), true
	}
	var caller models.User
	if err := database.DB.First(&caller, userID).Error; err != nil {
		return caller, false
	}
	if caller.Role == "manager" || (caller.Email != "" && IsEmailManager(caller.Email)) {
		return caller, true
	}
	return caller, false
}

// MemberSummaryDTO represents a unique person in the Member Management view
type MemberSummaryDTO struct {
	Key         string   `json:"key"`
	PrimaryID   uint     `json:"primary_id"`
	UserIDs     []uint   `json:"user_ids"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Ward        string   `json:"ward"`
	Roles       []string `json:"roles"`
	HighestRole string   `json:"highest_role"`
}

// ListMembers returns all members grouped by email (excluding the invisible secret super-admin)
func (h *ManagerHandler) ListMembers(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	var users []models.User
	database.DB.Order("id asc").Find(&users)

	memberMap := make(map[string]*MemberSummaryDTO)
	var orderedKeys []string

	for _, u := range users {
		if strings.EqualFold(strings.TrimSpace(u.Email), SecretAdminUsername) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(u.Email))
		if key == "" {
			key = u.WalletAddress
		}
		m, exists := memberMap[key]
		if !exists {
			m = &MemberSummaryDTO{
				Key:         key,
				PrimaryID:   u.ID,
				Name:        u.Name,
				Email:       u.Email,
				Ward:        u.Ward,
				HighestRole: "learner",
			}
			memberMap[key] = m
			orderedKeys = append(orderedKeys, key)
		}
		m.UserIDs = append(m.UserIDs, u.ID)
		if u.Name != "" && m.Name == "" {
			m.Name = u.Name
		}
		if u.Ward != "" {
			m.Ward = u.Ward
		}
		r := strings.ToLower(strings.TrimSpace(u.Role))
		if r == "" {
			r = "learner"
		}
		hasRole := false
		for _, existingR := range m.Roles {
			if existingR == r {
				hasRole = true
				break
			}
		}
		if !hasRole {
			m.Roles = append(m.Roles, r)
		}
		if r == "manager" {
			m.HighestRole = "manager"
		} else if r == "creator" && m.HighestRole != "manager" {
			m.HighestRole = "creator"
		}
	}

	members := make([]MemberSummaryDTO, 0, len(orderedKeys))
	for _, k := range orderedKeys {
		members = append(members, *memberMap[k])
	}

	return c.JSON(fiber.Map{
		"success": true,
		"members": members,
	})
}

// UpdateMemberRole promotes or demotes a member's access level (learner, creator, manager)
func (h *ManagerHandler) UpdateMemberRole(c *fiber.Ctx) error {
	caller, ok := requireManagerOrSecretAdmin(c)
	if !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	var req struct {
		Email      string `json:"email"`
		UserID     uint   `json:"user_id"`
		TargetRole string `json:"target_role"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
		})
	}

	targetRole := strings.ToLower(strings.TrimSpace(req.TargetRole))
	if targetRole != "learner" && targetRole != "creator" && targetRole != "manager" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "บทบาทไม่ถูกต้อง (ต้องเป็น learner, creator หรือ manager)",
		})
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(req.Email))
	var groupUsers []models.User
	if cleanEmail != "" {
		database.DB.Where("LOWER(email) = ?", cleanEmail).Find(&groupUsers)
	} else if req.UserID > 0 {
		var u models.User
		if err := database.DB.First(&u, req.UserID).Error; err == nil {
			if strings.TrimSpace(u.Email) != "" {
				cleanEmail = strings.ToLower(strings.TrimSpace(u.Email))
				database.DB.Where("LOWER(email) = ?", cleanEmail).Find(&groupUsers)
			} else {
				groupUsers = []models.User{u}
			}
		}
	}

	if len(groupUsers) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบข้อมูลสมาชิกที่ต้องการปรับสิทธิ์",
		})
	}

	if caller.ID != SecretAdminUserID && cleanEmail != "" && strings.EqualFold(caller.Email, cleanEmail) && targetRole != "manager" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถลดสิทธิ์บัญชีผู้จัดการของตนเองได้",
		})
	}

	base := groupUsers[0]
	for _, u := range groupUsers {
		if u.PasswordHash != "" {
			base.PasswordHash = u.PasswordHash
		}
		if u.Name != "" {
			base.Name = u.Name
		}
		if u.Ward != "" {
			base.Ward = u.Ward
		}
	}

	// Determine desired role set
	desiredRoles := []string{"learner"}
	if targetRole == "creator" {
		desiredRoles = []string{"learner", "creator"}
	} else if targetRole == "manager" {
		desiredRoles = []string{"learner", "creator", "manager"}
	}

	desiredMap := map[string]bool{}
	for _, r := range desiredRoles {
		desiredMap[r] = true
	}

	existingByRole := map[string]models.User{}
	for _, u := range groupUsers {
		r := strings.ToLower(strings.TrimSpace(u.Role))
		if r == "" {
			r = "learner"
		}
		if !desiredMap[r] {
			// Demote: remove higher role record
			database.DB.Unscoped().Delete(&u)
		} else {
			existingByRole[r] = u
		}
	}

	// Ensure each desired role exists
	for _, r := range desiredRoles {
		if _, exists := existingByRole[r]; !exists {
			wallet := base.WalletAddress
			if r != "learner" {
				key, _ := crypto.GenerateKey()
				wallet = crypto.PubkeyToAddress(key.PublicKey).Hex()
			}
			newUser := models.User{
				WalletAddress: wallet,
				Name:          base.Name,
				Email:         base.Email,
				PasswordHash:  base.PasswordHash,
				Ward:          base.Ward,
				Role:          r,
				AvatarURL:     base.AvatarURL,
			}
			database.DB.Create(&newUser)
		}
	}

	roleLabel := "ผู้เรียน"
	if targetRole == "creator" {
		roleLabel = "ผู้สร้างบทเรียน"
	} else if targetRole == "manager" {
		roleLabel = "ผู้จัดการ"
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("ปรับสิทธิ์ของ %s เป็น '%s' เรียบร้อยแล้ว", base.Name, roleLabel),
	})
}

// DeleteMember removes a member and all their associated learning records
func (h *ManagerHandler) DeleteMember(c *fiber.Ctx) error {
	caller, ok := requireManagerOrSecretAdmin(c)
	if !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	email := strings.ToLower(strings.TrimSpace(c.Query("email")))
	userIDStr := c.Query("user_id")
	userID, _ := strconv.ParseUint(userIDStr, 10, 32)

	var targets []models.User
	if email != "" {
		database.DB.Where("LOWER(email) = ?", email).Find(&targets)
	} else if userID > 0 {
		var u models.User
		if err := database.DB.First(&u, userID).Error; err == nil {
			if strings.TrimSpace(u.Email) != "" {
				email = strings.ToLower(strings.TrimSpace(u.Email))
				database.DB.Where("LOWER(email) = ?", email).Find(&targets)
			} else {
				targets = []models.User{u}
			}
		}
	}

	if len(targets) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบสมาชิกที่ต้องการลบ",
		})
	}

	for _, u := range targets {
		if u.ID == caller.ID || (caller.Email != "" && strings.EqualFold(caller.Email, u.Email)) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"message": "ไม่สามารถลบบัญชีของตนเองขณะกำลังใช้งานอยู่ได้",
			})
		}
	}

	deletedName := targets[0].Name
	for _, u := range targets {
		database.DB.Where("user_id = ?", u.ID).Delete(&models.LessonAssignment{})
		database.DB.Where("user_id = ?", u.ID).Delete(&models.UserLessonProgress{})
		database.DB.Where("user_id = ?", u.ID).Delete(&models.QuizAttempt{})
		database.DB.Where("user_id = ?", u.ID).Delete(&models.Certificate{})
		database.DB.Unscoped().Delete(&u)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("ลบสมาชิก '%s' ออกจากระบบเรียบร้อยแล้ว", deletedName),
	})
}

type LessonLearnerDetailDTO struct {
	UserID             uint      `json:"user_id"`
	Name               string    `json:"name"`
	Email              string    `json:"email"`
	Ward               string    `json:"ward"`
	IsAssigned         bool      `json:"is_assigned"`
	PreTestPassed      bool      `json:"pre_test_passed"`
	VideoWatched       bool      `json:"video_watched"`
	WatchedSec         float64   `json:"watched_sec"`
	PostTestPassed     bool      `json:"post_test_passed"`
	IsCompleted        bool      `json:"is_completed"`
	ScorePct           float64   `json:"score_pct"`
	CertificateTokenID string    `json:"certificate_token_id"`
	OnChainTxHash      string    `json:"on_chain_tx_hash"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// GetLessonLearnersStats lazy-loads per-learner statistics for a single lesson (Manager / Godess only)
func (h *ManagerHandler) GetLessonLearnersStats(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	lessonID, err := strconv.ParseUint(c.Params("lessonId"), 10, 32)
	if err != nil || lessonID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "รหัสบทเรียนไม่ถูกต้อง",
		})
	}
	wardFilter := strings.TrimSpace(c.Query("ward", ""))

	var lesson models.Lesson
	if err := database.DB.First(&lesson, lessonID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบบทเรียนที่ระบุ",
		})
	}

	var progs []models.UserLessonProgress
	database.DB.Where("lesson_id = ?", lesson.ID).Find(&progs)

	var assigns []models.LessonAssignment
	database.DB.Where("lesson_id = ?", lesson.ID).Find(&assigns)

	var certs []models.Certificate
	database.DB.Where("lesson_id = ?", lesson.ID).Find(&certs)

	var quizzes []models.Quiz
	database.DB.Where("lesson_id = ?", lesson.ID).Find(&quizzes)
	quizIDs := make([]uint, 0, len(quizzes))
	postQuizIDMap := make(map[uint]bool)
	for _, q := range quizzes {
		quizIDs = append(quizIDs, q.ID)
		if q.Type == "lesson_post_test" {
			postQuizIDMap[q.ID] = true
		}
	}

	var attempts []models.QuizAttempt
	if len(quizIDs) > 0 {
		database.DB.Where("quiz_id IN ?", quizIDs).Order("id desc").Find(&attempts)
	}

	progByUser := make(map[uint]models.UserLessonProgress, len(progs))
	userIDSet := make(map[uint]bool)
	for _, p := range progs {
		if p.UserID == SecretAdminUserID {
			continue
		}
		progByUser[p.UserID] = p
		userIDSet[p.UserID] = true
	}

	assignByUser := make(map[uint]bool, len(assigns))
	for _, a := range assigns {
		if a.UserID == SecretAdminUserID {
			continue
		}
		assignByUser[a.UserID] = true
		userIDSet[a.UserID] = true
	}

	certByUser := make(map[uint]models.Certificate, len(certs))
	for _, cert := range certs {
		if cert.UserID == SecretAdminUserID {
			continue
		}
		certByUser[cert.UserID] = cert
		userIDSet[cert.UserID] = true
	}

	bestScoreByUser := make(map[uint]float64)
	for _, att := range attempts {
		if att.TotalScore <= 0 {
			continue
		}
		pct := (att.Score / att.TotalScore) * 100.0
		// Prefer post-test score, or highest score if no post-test
		if postQuizIDMap[att.QuizID] || pct > bestScoreByUser[att.UserID] {
			if _, exists := bestScoreByUser[att.UserID]; !exists || postQuizIDMap[att.QuizID] || pct > bestScoreByUser[att.UserID] {
				bestScoreByUser[att.UserID] = pct
			}
		}
	}

	userIDs := make([]uint, 0, len(userIDSet))
	for uid := range userIDSet {
		userIDs = append(userIDs, uid)
	}

	var users []models.User
	if len(userIDs) > 0 {
		database.DB.Where("id IN ?", userIDs).Order("name asc").Find(&users)
	}

	learners := make([]LessonLearnerDetailDTO, 0, len(users))
	for _, u := range users {
		ward := strings.TrimSpace(u.Ward)
		if ward == "" {
			ward = "สว.11"
		}
		if wardFilter != "" && ward != wardFilter {
			continue
		}
		p := progByUser[u.ID]
		cert := certByUser[u.ID]
		scorePct := bestScoreByUser[u.ID]
		if cert.AverageScore > 0 && scorePct == 0 {
			scorePct = cert.AverageScore
		}
		learners = append(learners, LessonLearnerDetailDTO{
			UserID:             u.ID,
			Name:               u.Name,
			Email:              u.Email,
			Ward:               ward,
			IsAssigned:         assignByUser[u.ID],
			PreTestPassed:      p.PreTestPassed,
			VideoWatched:       p.VideoWatched,
			WatchedSec:         p.LastWatchedSec,
			PostTestPassed:     p.PostTestPassed,
			IsCompleted:        p.IsCompleted,
			ScorePct:           scorePct,
			CertificateTokenID: cert.TokenID,
			OnChainTxHash:      p.OnChainTxHash,
			UpdatedAt:          p.UpdatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"lesson_id":      lesson.ID,
		"lesson_title":   lesson.Title,
		"duration_sec":   lesson.DurationSec,
		"total_learners": len(learners),
		"learners":       learners,
	})
}

type WardLearnerLessonDTO struct {
	LessonID           uint    `json:"lesson_id"`
	SortOrder          int     `json:"sort_order"`
	LessonTitle        string  `json:"lesson_title"`
	DurationSec        float64 `json:"duration_sec"`
	IsAssigned         bool    `json:"is_assigned"`
	PreTestPassed      bool    `json:"pre_test_passed"`
	VideoWatched       bool    `json:"video_watched"`
	WatchedSec         float64 `json:"watched_sec"`
	PostTestPassed     bool    `json:"post_test_passed"`
	IsCompleted        bool    `json:"is_completed"`
	ScorePct           float64 `json:"score_pct"`
	CertificateTokenID string  `json:"certificate_token_id"`
	StatusLabel        string  `json:"status_label"`
	StatusKey          string  `json:"status_key"` // completed, in_progress, not_started
}

type WardLearnerSummaryDTO struct {
	UserID          uint                   `json:"user_id"`
	Name            string                 `json:"name"`
	Email           string                 `json:"email"`
	Ward            string                 `json:"ward"`
	CompletedCount  int                    `json:"completed_count"`
	InProgressCount int                    `json:"in_progress_count"`
	NotStartedCount int                    `json:"not_started_count"`
	Lessons         []WardLearnerLessonDTO `json:"lessons"`
}

// GetWardLearnersStats lazy-loads who is studying what & their status in a specific ward (Manager / Godess only)
func (h *ManagerHandler) GetWardLearnersStats(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	rawWard := c.Params("ward")
	wardName, err := url.PathUnescape(rawWard)
	if err != nil {
		wardName = rawWard
	}
	wardName = strings.TrimSpace(wardName)
	if wardName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "กรุณาระบุชื่อหน่วยงาน",
		})
	}

	// Fetch users in this ward (excluding secret admin)
	var allUsers []models.User
	database.DB.Where("id != ?", SecretAdminUserID).Order("name asc").Find(&allUsers)
	wardUsers := make([]models.User, 0)
	userIDs := make([]uint, 0)
	for _, u := range allUsers {
		w := strings.TrimSpace(u.Ward)
		if w == "" {
			w = "สว.11"
		}
		if w == wardName {
			wardUsers = append(wardUsers, u)
			userIDs = append(userIDs, u.ID)
		}
	}

	// Fetch active (non-DISSOLVED) lessons
	var allLessons []models.Lesson
	database.DB.Order("sort_order asc, id asc").Find(&allLessons)
	lessonMap := make(map[uint]models.Lesson)
	for _, l := range allLessons {
		if !ComputeBlockLifecycle(l.CreatedAt).IsExpired {
			lessonMap[l.ID] = l
		}
	}

	if len(userIDs) == 0 {
		return c.JSON(fiber.Map{
			"success":       true,
			"ward":          wardName,
			"total_members": 0,
			"members":       []WardLearnerSummaryDTO{},
		})
	}

	var progs []models.UserLessonProgress
	database.DB.Where("user_id IN ?", userIDs).Find(&progs)

	var assigns []models.LessonAssignment
	database.DB.Where("user_id IN ?", userIDs).Find(&assigns)

	var certs []models.Certificate
	database.DB.Where("user_id IN ?", userIDs).Find(&certs)

	var quizzes []models.Quiz
	database.DB.Find(&quizzes)
	quizLessonMap := make(map[uint]uint, len(quizzes))
	postQuizMap := make(map[uint]bool)
	for _, q := range quizzes {
		quizLessonMap[q.ID] = q.LessonID
		if q.Type == "lesson_post_test" {
			postQuizMap[q.ID] = true
		}
	}

	var attempts []models.QuizAttempt
	database.DB.Where("user_id IN ?", userIDs).Order("id desc").Find(&attempts)

	type ulKey struct {
		uid uint
		lid uint
	}
	progMap := make(map[ulKey]models.UserLessonProgress)
	userLessonSet := make(map[uint]map[uint]bool)
	addUL := func(uid, lid uint) {
		if _, ok := lessonMap[lid]; !ok {
			return
		}
		if userLessonSet[uid] == nil {
			userLessonSet[uid] = make(map[uint]bool)
		}
		userLessonSet[uid][lid] = true
	}

	for _, p := range progs {
		progMap[ulKey{p.UserID, p.LessonID}] = p
		if p.PreTestPassed || p.VideoWatched || p.LastWatchedSec > 0 || p.PostTestPassed || p.IsCompleted {
			addUL(p.UserID, p.LessonID)
		}
	}
	assignMap := make(map[ulKey]bool)
	for _, a := range assigns {
		assignMap[ulKey{a.UserID, a.LessonID}] = true
		addUL(a.UserID, a.LessonID)
	}
	certMap := make(map[ulKey]models.Certificate)
	for _, cert := range certs {
		certMap[ulKey{cert.UserID, cert.LessonID}] = cert
		addUL(cert.UserID, cert.LessonID)
	}
	scoreMap := make(map[ulKey]float64)
	for _, att := range attempts {
		lid := quizLessonMap[att.QuizID]
		if lid == 0 || att.TotalScore <= 0 {
			continue
		}
		pct := (att.Score / att.TotalScore) * 100.0
		k := ulKey{att.UserID, lid}
		if _, exists := scoreMap[k]; !exists || postQuizMap[att.QuizID] || pct > scoreMap[k] {
			scoreMap[k] = pct
		}
	}

	// Group by email (or user_id if no email) so the same person isn't duplicated across roles
	type personGroup struct {
		dto       WardLearnerSummaryDTO
		lessonSet map[uint]bool
	}
	groupOrder := make([]string, 0)
	groups := make(map[string]*personGroup)

	for _, u := range wardUsers {
		key := strings.ToLower(strings.TrimSpace(u.Email))
		if key == "" {
			key = fmt.Sprintf("id:%d", u.ID)
		}
		grp, exists := groups[key]
		if !exists {
			grp = &personGroup{
				dto: WardLearnerSummaryDTO{
					UserID:  u.ID,
					Name:    u.Name,
					Email:   u.Email,
					Ward:    wardName,
					Lessons: []WardLearnerLessonDTO{},
				},
				lessonSet: make(map[uint]bool),
			}
			groups[key] = grp
			groupOrder = append(groupOrder, key)
		}

		for lid := range userLessonSet[u.ID] {
			if grp.lessonSet[lid] {
				continue
			}
			grp.lessonSet[lid] = true
			l := lessonMap[lid]
			k := ulKey{u.ID, lid}
			p := progMap[k]
			cert := certMap[k]
			isAssigned := assignMap[k]
			scorePct := scoreMap[k]
			if cert.AverageScore > 0 && scorePct == 0 {
				scorePct = cert.AverageScore
			}

			statusKey := "not_started"
			statusLabel := "📌 มอบหมาย (ยังไม่เริ่ม)"
			if p.IsCompleted {
				statusKey = "completed"
				statusLabel = "✅ เรียนจบ & ได้วุฒิบัตร"
				grp.dto.CompletedCount++
			} else if p.VideoWatched {
				statusKey = "in_progress"
				statusLabel = "📝 รอสอบหลังเรียน"
				grp.dto.InProgressCount++
			} else if p.PreTestPassed || p.LastWatchedSec > 0 {
				statusKey = "in_progress"
				statusLabel = "🎬 กำลังดูวิดีโอ"
				grp.dto.InProgressCount++
			} else {
				grp.dto.NotStartedCount++
			}

			grp.dto.Lessons = append(grp.dto.Lessons, WardLearnerLessonDTO{
				LessonID:           l.ID,
				SortOrder:          l.SortOrder,
				LessonTitle:        l.Title,
				DurationSec:        l.DurationSec,
				IsAssigned:         isAssigned,
				PreTestPassed:      p.PreTestPassed,
				VideoWatched:       p.VideoWatched,
				WatchedSec:         p.LastWatchedSec,
				PostTestPassed:     p.PostTestPassed,
				IsCompleted:        p.IsCompleted,
				ScorePct:           scorePct,
				CertificateTokenID: cert.TokenID,
				StatusLabel:        statusLabel,
				StatusKey:          statusKey,
			})
		}
	}

	result := make([]WardLearnerSummaryDTO, 0, len(groupOrder))
	for _, key := range groupOrder {
		result = append(result, groups[key].dto)
	}

	return c.JSON(fiber.Map{
		"success":       true,
		"ward":          wardName,
		"total_members": len(result),
		"members":       result,
	})
}

type ReportedCommentDTO struct {
	ID           uint       `json:"id"`
	LessonID     uint       `json:"lesson_id"`
	LessonTitle  string     `json:"lesson_title"`
	Rating       int        `json:"rating"`
	Comment      string     `json:"comment"`
	ReportReason string     `json:"report_reason"`
	ReportedBy   string     `json:"reported_by"`
	ReportedAt   *time.Time `json:"reported_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ListReportedComments returns all learner comments flagged by creators for manager review.
func (h *ManagerHandler) ListReportedComments(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	var ratings []models.LessonRating
	database.DB.Where("is_reported = ? AND comment != ''", true).Order("reported_at desc, updated_at desc").Find(&ratings)

	var lessons []models.Lesson
	database.DB.Find(&lessons)
	lessonTitleMap := make(map[uint]string, len(lessons))
	for _, l := range lessons {
		lessonTitleMap[l.ID] = l.Title
	}

	out := make([]ReportedCommentDTO, 0, len(ratings))
	for _, r := range ratings {
		out = append(out, ReportedCommentDTO{
			ID:           r.ID,
			LessonID:     r.LessonID,
			LessonTitle:  lessonTitleMap[r.LessonID],
			Rating:       r.Rating,
			Comment:      r.Comment,
			ReportReason: r.ReportReason,
			ReportedBy:   r.ReportedBy,
			ReportedAt:   r.ReportedAt,
			CreatedAt:    r.UpdatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"comments": out,
	})
}

// DeleteReportedComment clears an inappropriate learner comment while keeping the anonymous star rating.
func (h *ManagerHandler) DeleteReportedComment(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	ratingID, err := strconv.ParseUint(c.Params("ratingId"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "รหัสความคิดเห็นไม่ถูกต้อง",
		})
	}

	now := time.Now()
	if err := database.DB.Model(&models.LessonRating{}).Where("id = ?", uint(ratingID)).Updates(map[string]interface{}{
		"comment":              "",
		"is_reported":          false,
		"report_reason":        "",
		"reported_by":          "",
		"reported_at":          nil,
		"comment_moderated_at": &now,
	}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถลบข้อความได้",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ลบข้อความไม่เหมาะสมเรียบร้อยแล้ว",
	})
}

// DismissReportedComment clears the report flag while keeping the comment.
func (h *ManagerHandler) DismissReportedComment(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	ratingID, err := strconv.ParseUint(c.Params("ratingId"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "รหัสความคิดเห็นไม่ถูกต้อง",
		})
	}

	if err := database.DB.Model(&models.LessonRating{}).Where("id = ?", uint(ratingID)).Updates(map[string]interface{}{
		"is_reported":   false,
		"report_reason": "",
		"reported_by":   "",
		"reported_at":   nil,
	}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถยกเลิกการแจ้งได้",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ยกเลิกการแจ้งลบแล้ว",
	})
}
