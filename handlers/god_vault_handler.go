package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"elearning-sbt/database"
	"elearning-sbt/middleware"
	"elearning-sbt/models"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GodVaultHandler struct{}

func NewGodVaultHandler() *GodVaultHandler {
	return &GodVaultHandler{}
}

const (
	vaultRootDir        = "vault"
	vaultColdOffloadDir = "vault/cold_offload"
	vaultSnapshotsDir   = "vault/snapshots"
	vaultDistilledDir   = "vault/distilled"
)

func ensureVaultDirs() error {
	for _, dir := range []string{vaultRootDir, vaultColdOffloadDir, vaultSnapshotsDir, vaultDistilledDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// requireGodTherunrock strictly allows ONLY SecretAdminUserID (therunrock / Godess)
func requireGodTherunrock(c *fiber.Ctx) bool {
	return middleware.ResolveUserID(c) == SecretAdminUserID
}

func computeSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func computeMerkleRoot(leaves []string) string {
	if len(leaves) == 0 {
		return computeSHA256Hex([]byte("EMPTY_MERKLE_TREE"))
	}
	level := make([]string, len(leaves))
	copy(level, leaves)
	for len(level) > 1 {
		if len(level)%2 == 1 {
			level = append(level, level[len(level)-1])
		}
		next := make([]string, 0, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			combined := computeSHA256Hex([]byte(level[i] + "|" + level[i+1]))
			next = append(next, combined)
		}
		level = next
	}
	return level[0]
}

type VaultEnvelope struct {
	VaultType     string          `json:"vault_type"` // COLD_OFFLOAD, STATE_SNAPSHOT, DATA_DISTILLATION
	Codename      string          `json:"codename"`
	Action        string          `json:"action"`
	CreatedAt     time.Time       `json:"created_at"`
	CreatedBy     string          `json:"created_by"`
	MerkleRoot    string          `json:"merkle_root"`
	PayloadSHA256 string          `json:"payload_sha256"`
	Payload       json.RawMessage `json:"payload"`
}

type ColdOffloadPayload struct {
	Lesson      models.Lesson               `json:"lesson"`
	Quizzes     []models.Quiz               `json:"quizzes"`
	Questions   []models.Question           `json:"questions"`
	Options     []models.Option             `json:"options"`
	Progress    []models.UserLessonProgress `json:"progress"`
	Attempts    []models.QuizAttempt        `json:"attempts"`
	Assignments []models.LessonAssignment   `json:"assignments"`
	Certs       []models.Certificate        `json:"certificates"`
}

type SnapshotUser struct {
	models.User
	PasswordHash string `json:"password_hash,omitempty"`
}

type StateSnapshotPayload struct {
	Users       []SnapshotUser              `json:"users"`
	Courses     []models.Course             `json:"courses"`
	Lessons     []models.Lesson             `json:"lessons"`
	Quizzes     []models.Quiz               `json:"quizzes"`
	Questions   []models.Question           `json:"questions"`
	Options     []models.Option             `json:"options"`
	Assignments []models.LessonAssignment   `json:"assignments"`
	Progress    []models.UserLessonProgress `json:"progress"`
	Attempts    []models.QuizAttempt        `json:"attempts"`
	Certs       []models.Certificate        `json:"certificates"`
}

type DistilledEssencePayload struct {
	LessonIDFilter uint     `json:"lesson_id_filter"`
	LeafCount      int      `json:"leaf_count"`
	CompletedCount int      `json:"completed_count"`
	CertsCount     int      `json:"certs_count"`
	AttemptsCount  int      `json:"attempts_count"`
	MerkleRoot     string   `json:"merkle_root"`
	LeafHashes     []string `json:"leaf_hashes"`
}

type VaultFileDTO struct {
	Filename   string    `json:"filename"`
	Category   string    `json:"category"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

func listVaultFiles(dir string, category string) []VaultFileDTO {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []VaultFileDTO{}
	}
	files := make([]VaultFileDTO, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, VaultFileDTO{
			Filename:   e.Name(),
			Category:   category,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt.After(files[j].ModifiedAt)
	})
	return files
}

func writeVaultEnvelope(dir, filename, vaultType, codename, action, merkleRoot string, payloadObj interface{}) (string, VaultEnvelope, int64, error) {
	if err := ensureVaultDirs(); err != nil {
		return "", VaultEnvelope{}, 0, err
	}
	payloadBytes, err := json.Marshal(payloadObj)
	if err != nil {
		return "", VaultEnvelope{}, 0, err
	}
	env := VaultEnvelope{
		VaultType:     vaultType,
		Codename:      codename,
		Action:        action,
		CreatedAt:     time.Now(),
		CreatedBy:     "Godess (therunrock)",
		MerkleRoot:    merkleRoot,
		PayloadSHA256: computeSHA256Hex(payloadBytes),
		Payload:       json.RawMessage(payloadBytes),
	}
	fullBytes, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return "", VaultEnvelope{}, 0, err
	}
	fullPath := filepath.Join(dir, filepath.Base(filename))
	if err := os.WriteFile(fullPath, fullBytes, 0644); err != nil {
		return "", VaultEnvelope{}, 0, err
	}
	return fullPath, env, int64(len(fullBytes)), nil
}

func readAndVerifyEnvelope(fullPath string) (VaultEnvelope, bool, string, error) {
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return VaultEnvelope{}, false, "", err
	}
	var env VaultEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return VaultEnvelope{}, false, "", err
	}
	var compactBuf bytes.Buffer
	if err := json.Compact(&compactBuf, env.Payload); err != nil {
		return env, false, "", err
	}
	actualHash := computeSHA256Hex(compactBuf.Bytes())
	intact := actualHash != "" && strings.EqualFold(actualHash, env.PayloadSHA256)
	return env, intact, actualHash, nil
}

// GetVaultStatus returns active chain metrics & all off-chain vault files (God Therunrock only)
func (h *GodVaultHandler) GetVaultStatus(c *fiber.Ctx) error {
	if !requireGodTherunrock(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "⚡ สิทธิ์ถูกปฏิเสธ: เฉพาะ God Therunrock เท่านั้นที่เข้าถึงห้องนิรภัยนี้ได้",
		})
	}
	_ = ensureVaultDirs()

	var dbSizeBytes int64
	if info, err := os.Stat("elearning.db"); err == nil {
		dbSizeBytes = info.Size()
	}

	var usersCount, coursesCount, lessonsCount, progressCount, attemptsCount, certsCount int64
	database.DB.Model(&models.User{}).Count(&usersCount)
	database.DB.Model(&models.Course{}).Count(&coursesCount)
	database.DB.Model(&models.Lesson{}).Count(&lessonsCount)
	database.DB.Model(&models.UserLessonProgress{}).Count(&progressCount)
	database.DB.Model(&models.QuizAttempt{}).Count(&attemptsCount)
	database.DB.Model(&models.Certificate{}).Count(&certsCount)

	var lessons []models.Lesson
	database.DB.Order("id asc").Find(&lessons)
	type simpleLesson struct {
		ID      uint   `json:"id"`
		Title   string `json:"title"`
		Stage   string `json:"stage"`
		AgeDays int    `json:"age_days"`
	}
	lessonList := make([]simpleLesson, 0, len(lessons))
	for _, l := range lessons {
		lc := ComputeBlockLifecycle(l.CreatedAt)
		lessonList = append(lessonList, simpleLesson{
			ID:      l.ID,
			Title:   l.Title,
			Stage:   lc.Stage,
			AgeDays: lc.AgeDays,
		})
	}

	return c.JSON(fiber.Map{
		"success":       true,
		"db_size_bytes": dbSizeBytes,
		"counts": fiber.Map{
			"users":         usersCount,
			"courses":       coursesCount,
			"lessons":       lessonsCount,
			"progress":      progressCount,
			"quiz_attempts": attemptsCount,
			"certificates":  certsCount,
		},
		"active_lessons":     lessonList,
		"cold_offload_files": listVaultFiles(vaultColdOffloadDir, "cold_offload"),
		"snapshot_files":     listVaultFiles(vaultSnapshotsDir, "snapshots"),
		"distilled_files":    listVaultFiles(vaultDistilledDir, "distilled"),
	})
}

// 1. ETHEREAL VAULT — Action: COLD OFFLOAD
func (h *GodVaultHandler) ColdOffload(c *fiber.Ctx) error {
	if !requireGodTherunrock(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "เฉพาะ God Therunrock เท่านั้น"})
	}
	var req struct {
		LessonID uint `json:"lesson_id"`
	}
	_ = c.BodyParser(&req)

	var lesson models.Lesson
	if req.LessonID > 0 {
		if err := database.DB.First(&lesson, req.LessonID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "ไม่พบบทเรียนที่ระบุ"})
		}
	} else {
		if err := database.DB.Order("created_at asc").First(&lesson).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "ไม่มีบทเรียนในระบบสำหรับ Cold Offload"})
		}
	}

	var quizzes []models.Quiz
	database.DB.Where("lesson_id = ?", lesson.ID).Find(&quizzes)
	quizIDs := make([]uint, 0, len(quizzes))
	for _, q := range quizzes {
		quizIDs = append(quizIDs, q.ID)
	}

	var questions []models.Question
	if len(quizIDs) > 0 {
		database.DB.Where("quiz_id IN ?", quizIDs).Find(&questions)
	}
	questionIDs := make([]uint, 0, len(questions))
	for _, qu := range questions {
		questionIDs = append(questionIDs, qu.ID)
	}

	var options []models.Option
	if len(questionIDs) > 0 {
		database.DB.Where("question_id IN ?", questionIDs).Find(&options)
	}

	var progress []models.UserLessonProgress
	database.DB.Where("lesson_id = ?", lesson.ID).Find(&progress)

	var attempts []models.QuizAttempt
	if len(quizIDs) > 0 {
		database.DB.Where("quiz_id IN ?", quizIDs).Find(&attempts)
	}

	var assignments []models.LessonAssignment
	database.DB.Where("lesson_id = ?", lesson.ID).Find(&assignments)

	var certs []models.Certificate
	database.DB.Where("lesson_id = ?", lesson.ID).Find(&certs)

	leaves := []string{computeSHA256Hex([]byte(fmt.Sprintf("LESSON|%d|%s", lesson.ID, lesson.Title)))}
	for _, p := range progress {
		leaves = append(leaves, computeSHA256Hex([]byte(fmt.Sprintf("PROG|%d|%d|%t|%s", p.UserID, p.LessonID, p.IsCompleted, p.OnChainTxHash))))
	}
	for _, cert := range certs {
		leaves = append(leaves, computeSHA256Hex([]byte(fmt.Sprintf("CERT|%s|%s", cert.TokenID, cert.TransactionHash))))
	}
	merkleRoot := computeMerkleRoot(leaves)

	payload := ColdOffloadPayload{
		Lesson:      lesson,
		Quizzes:     quizzes,
		Questions:   questions,
		Options:     options,
		Progress:    progress,
		Attempts:    attempts,
		Assignments: assignments,
		Certs:       certs,
	}

	filename := fmt.Sprintf("offload_lesson_%d_%s.json", lesson.ID, time.Now().Format("20060102_150405"))
	_, env, sizeBytes, err := writeVaultEnvelope(
		vaultColdOffloadDir, filename,
		"COLD_OFFLOAD", "ETHEREAL VAULT", "COLD OFFLOAD",
		merkleRoot, payload,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	// Remove offloaded lesson records from active SQLite to keep main chain fast
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if len(questionIDs) > 0 {
			tx.Unscoped().Where("question_id IN ?", questionIDs).Delete(&models.Option{})
		}
		if len(quizIDs) > 0 {
			tx.Unscoped().Where("quiz_id IN ?", quizIDs).Delete(&models.Question{})
			tx.Unscoped().Where("quiz_id IN ?", quizIDs).Delete(&models.QuizAttempt{})
		}
		tx.Unscoped().Where("lesson_id = ?", lesson.ID).Delete(&models.Quiz{})
		tx.Unscoped().Where("lesson_id = ?", lesson.ID).Delete(&models.LessonAssignment{})
		tx.Unscoped().Where("lesson_id = ?", lesson.ID).Delete(&models.UserLessonProgress{})
		tx.Unscoped().Where("lesson_id = ?", lesson.ID).Delete(&models.Certificate{})
		return tx.Unscoped().Delete(&lesson).Error
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":     true,
		"codename":    "ETHEREAL VAULT",
		"action":      "COLD OFFLOAD",
		"filename":    filename,
		"size_bytes":  sizeBytes,
		"sha256":      env.PayloadSHA256,
		"merkle_root": env.MerkleRoot,
		"message":     fmt.Sprintf("🏛️ ETHEREAL VAULT — ย้ายบทเรียน '%s' ไปเก็บในห้องนิรภัยนอกเชน (%s) เรียบร้อยแล้ว", lesson.Title, filename),
	})
}

// 2. ECHO RECALL — Action: STATE REHYDRATE
func (h *GodVaultHandler) StateRehydrate(c *fiber.Ctx) error {
	if !requireGodTherunrock(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "เฉพาะ God Therunrock เท่านั้น"})
	}
	var req struct {
		Filename string `json:"filename"`
		Unhide   bool   `json:"unhide"`
	}
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Filename) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "กรุณาระบุชื่อไฟล์จากห้องนิรภัย"})
	}

	safeName := filepath.Base(req.Filename)
	fullPath := filepath.Join(vaultColdOffloadDir, safeName)
	env, intact, actualHash, err := readAndVerifyEnvelope(fullPath)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "ไม่พบไฟล์ห้องนิรภัย: " + err.Error()})
	}
	if !intact {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": fmt.Sprintf("CRYSTAL CHECK FAILED: ไฟล์ %s ไม่สมบูรณ์ (expected %s, got %s)", safeName, env.PayloadSHA256, actualHash),
		})
	}

	var payload ColdOffloadPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "รูปแบบไฟล์ไม่ถูกต้อง: " + err.Error()})
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		payload.Lesson.DeletedAt = gorm.DeletedAt{}
		if req.Unhide {
			payload.Lesson.IsHidden = false
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&payload.Lesson).Error; err != nil {
			return err
		}
		for _, q := range payload.Quizzes {
			q.DeletedAt = gorm.DeletedAt{}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&q).Error; err != nil {
				return err
			}
		}
		for _, qu := range payload.Questions {
			qu.DeletedAt = gorm.DeletedAt{}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&qu).Error; err != nil {
				return err
			}
		}
		for _, op := range payload.Options {
			op.DeletedAt = gorm.DeletedAt{}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&op).Error; err != nil {
				return err
			}
		}
		for _, p := range payload.Progress {
			p.DeletedAt = gorm.DeletedAt{}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&p).Error; err != nil {
				return err
			}
		}
		for _, a := range payload.Attempts {
			a.DeletedAt = gorm.DeletedAt{}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&a).Error; err != nil {
				return err
			}
		}
		for _, as := range payload.Assignments {
			as.DeletedAt = gorm.DeletedAt{}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&as).Error; err != nil {
				return err
			}
		}
		for _, cert := range payload.Certs {
			cert.DeletedAt = gorm.DeletedAt{}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&cert).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"codename":  "ECHO RECALL",
		"action":    "STATE REHYDRATE",
		"lesson_id": payload.Lesson.ID,
		"title":     payload.Lesson.Title,
		"message":   fmt.Sprintf("🔮 ECHO RECALL — ปลุกบทเรียน '%s' จาก %s กลับคืนสู่เชนหลักอย่างไร้รอยต่อแล้ว", payload.Lesson.Title, safeName),
	})
}

// 3. OBLIVION SWEEP — Action: EXPIRED PRUNE
func (h *GodVaultHandler) OblivionSweep(c *fiber.Ctx) error {
	if !requireGodTherunrock(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "เฉพาะ God Therunrock เท่านั้น"})
	}

	expiredLessons, err := collectExpiredLessons(0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	prunedCount := 0
	for _, l := range expiredLessons {
		var quizzes []models.Quiz
		database.DB.Where("lesson_id = ?", l.ID).Find(&quizzes)
		for _, q := range quizzes {
			var questions []models.Question
			database.DB.Where("quiz_id = ?", q.ID).Find(&questions)
			for _, qu := range questions {
				database.DB.Unscoped().Where("question_id = ?", qu.ID).Delete(&models.Option{})
			}
			database.DB.Unscoped().Where("quiz_id = ?", q.ID).Delete(&models.Question{})
			database.DB.Unscoped().Where("quiz_id = ?", q.ID).Delete(&models.QuizAttempt{})
		}
		database.DB.Unscoped().Where("lesson_id = ?", l.ID).Delete(&models.Quiz{})
		database.DB.Unscoped().Where("lesson_id = ?", l.ID).Delete(&models.LessonAssignment{})
		database.DB.Unscoped().Where("lesson_id = ?", l.ID).Delete(&models.UserLessonProgress{})
		database.DB.Unscoped().Where("lesson_id = ?", l.ID).Delete(&models.Certificate{})
		database.DB.Unscoped().Delete(&l)
		prunedCount++
	}

	// Reclaim SQLite disk space
	_ = database.DB.Exec("VACUUM").Error
	var dbSizeBytes int64
	if info, err := os.Stat("elearning.db"); err == nil {
		dbSizeBytes = info.Size()
	}

	return c.JSON(fiber.Map{
		"success":       true,
		"codename":      "OBLIVION SWEEP",
		"action":        "EXPIRED PRUNE",
		"pruned_count":  prunedCount,
		"db_size_bytes": dbSizeBytes,
		"message":       fmt.Sprintf("🌑 OBLIVION SWEEP — กวาดล้างบล็อกหมดอายุ %d บล็อกและบีบอัดพื้นที่ฐานข้อมูลเรียบร้อย (ขนาด DB ปัจจุบัน: %d bytes)", prunedCount, dbSizeBytes),
	})
}

// 4. MIRROR REFLECTION — Action: STATE SNAPSHOT
func (h *GodVaultHandler) StateSnapshot(c *fiber.Ctx) error {
	if !requireGodTherunrock(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "เฉพาะ God Therunrock เท่านั้น"})
	}

	var users []models.User
	database.DB.Order("id asc").Find(&users)
	snapUsers := make([]SnapshotUser, 0, len(users))
	for _, u := range users {
		snapUsers = append(snapUsers, SnapshotUser{User: u, PasswordHash: u.PasswordHash})
	}

	var courses []models.Course
	database.DB.Order("id asc").Find(&courses)
	var lessons []models.Lesson
	database.DB.Order("id asc").Find(&lessons)
	var quizzes []models.Quiz
	database.DB.Order("id asc").Find(&quizzes)
	var questions []models.Question
	database.DB.Order("id asc").Find(&questions)
	var options []models.Option
	database.DB.Order("id asc").Find(&options)
	var assignments []models.LessonAssignment
	database.DB.Order("id asc").Find(&assignments)
	var progress []models.UserLessonProgress
	database.DB.Order("id asc").Find(&progress)
	var attempts []models.QuizAttempt
	database.DB.Order("id asc").Find(&attempts)
	var certs []models.Certificate
	database.DB.Order("id asc").Find(&certs)

	leaves := make([]string, 0, len(users)+len(lessons)+len(progress)+len(certs))
	for _, u := range users {
		leaves = append(leaves, computeSHA256Hex([]byte(fmt.Sprintf("USER|%d|%s|%s", u.ID, u.WalletAddress, u.Role))))
	}
	for _, l := range lessons {
		leaves = append(leaves, computeSHA256Hex([]byte(fmt.Sprintf("LESSON|%d|%s|%d", l.ID, l.Title, l.CreatedAt.Unix()))))
	}
	for _, p := range progress {
		leaves = append(leaves, computeSHA256Hex([]byte(fmt.Sprintf("PROG|%d|%d|%t|%s", p.UserID, p.LessonID, p.IsCompleted, p.OnChainTxHash))))
	}
	for _, cert := range certs {
		leaves = append(leaves, computeSHA256Hex([]byte(fmt.Sprintf("CERT|%s|%s", cert.TokenID, cert.TransactionHash))))
	}
	merkleRoot := computeMerkleRoot(leaves)

	payload := StateSnapshotPayload{
		Users:       snapUsers,
		Courses:     courses,
		Lessons:     lessons,
		Quizzes:     quizzes,
		Questions:   questions,
		Options:     options,
		Assignments: assignments,
		Progress:    progress,
		Attempts:    attempts,
		Certs:       certs,
	}

	filename := fmt.Sprintf("snapshot_%s.json", time.Now().Format("20060102_150405"))
	_, env, sizeBytes, err := writeVaultEnvelope(
		vaultSnapshotsDir, filename,
		"STATE_SNAPSHOT", "MIRROR REFLECTION", "STATE SNAPSHOT",
		merkleRoot, payload,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":     true,
		"codename":    "MIRROR REFLECTION",
		"action":      "STATE SNAPSHOT",
		"filename":    filename,
		"size_bytes":  sizeBytes,
		"sha256":      env.PayloadSHA256,
		"merkle_root": env.MerkleRoot,
		"counts": fiber.Map{
			"users":        len(users),
			"lessons":      len(lessons),
			"progress":     len(progress),
			"certificates": len(certs),
		},
		"message": fmt.Sprintf("🪞 MIRROR REFLECTION — บันทึกภาพสะท้อนสถานะทั้งเชน (%s) สำเร็จ พร้อม SHA-256 & Merkle Root", filename),
	})
}

// 5. PURE ESSENCE — Action: DATA DISTILLATION (Merkle Proof compression without deleting)
func (h *GodVaultHandler) DataDistillation(c *fiber.Ctx) error {
	if !requireGodTherunrock(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "เฉพาะ God Therunrock เท่านั้น"})
	}
	var req struct {
		LessonID uint `json:"lesson_id"`
	}
	_ = c.BodyParser(&req)

	var progress []models.UserLessonProgress
	var attempts []models.QuizAttempt
	var certs []models.Certificate

	if req.LessonID > 0 {
		database.DB.Where("lesson_id = ?", req.LessonID).Find(&progress)
		database.DB.Table("quiz_attempts").Joins("JOIN quizzes ON quizzes.id = quiz_attempts.quiz_id").Where("quizzes.lesson_id = ?", req.LessonID).Find(&attempts)
		database.DB.Where("lesson_id = ?", req.LessonID).Find(&certs)
	} else {
		database.DB.Find(&progress)
		database.DB.Find(&attempts)
		database.DB.Find(&certs)
	}

	rawObj := fiber.Map{"progress": progress, "attempts": attempts, "certs": certs}
	rawBytes, _ := json.Marshal(rawObj)

	leafHashes := make([]string, 0, len(progress)+len(attempts)+len(certs))
	completedCount := 0
	for _, p := range progress {
		if p.IsCompleted {
			completedCount++
		}
		leafHashes = append(leafHashes, computeSHA256Hex([]byte(fmt.Sprintf("P:%d:%d:%t:%t:%t:%s", p.UserID, p.LessonID, p.PreTestPassed, p.VideoWatched, p.PostTestPassed, p.OnChainTxHash))))
	}
	for _, a := range attempts {
		leafHashes = append(leafHashes, computeSHA256Hex([]byte(fmt.Sprintf("A:%d:%d:%.1f/%.1f:%t:%s", a.UserID, a.QuizID, a.Score, a.TotalScore, a.IsPassed, a.OnChainTxHash))))
	}
	for _, cert := range certs {
		leafHashes = append(leafHashes, computeSHA256Hex([]byte(fmt.Sprintf("C:%d:%d:%s:%s", cert.UserID, cert.LessonID, cert.TokenID, cert.TransactionHash))))
	}

	merkleRoot := computeMerkleRoot(leafHashes)
	distilled := DistilledEssencePayload{
		LessonIDFilter: req.LessonID,
		LeafCount:      len(leafHashes),
		CompletedCount: completedCount,
		CertsCount:     len(certs),
		AttemptsCount:  len(attempts),
		MerkleRoot:     merkleRoot,
		LeafHashes:     leafHashes,
	}

	filename := fmt.Sprintf("distill_%s.json", time.Now().Format("20060102_150405"))
	_, env, distilledBytes, err := writeVaultEnvelope(
		vaultDistilledDir, filename,
		"DATA_DISTILLATION", "PURE ESSENCE", "DATA DISTILLATION",
		merkleRoot, distilled,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":         true,
		"codename":        "PURE ESSENCE",
		"action":          "DATA DISTILLATION",
		"filename":        filename,
		"merkle_root":     merkleRoot,
		"sha256":          env.PayloadSHA256,
		"leaf_count":      len(leafHashes),
		"raw_bytes":       len(rawBytes),
		"distilled_bytes": distilledBytes,
		"message":         fmt.Sprintf("💎 PURE ESSENCE — กลั่นกรองข้อมูล %d รายการเป็น Merkle Root (%s...) สำเร็จ", len(leafHashes), merkleRoot[:16]),
	})
}

// 6. CRYSTAL AUDIT — Action: INTEGRITY VERIFY
func (h *GodVaultHandler) CrystalAudit(c *fiber.Ctx) error {
	if !requireGodTherunrock(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "เฉพาะ God Therunrock เท่านั้น"})
	}
	_ = ensureVaultDirs()

	type AuditItem struct {
		Filename       string `json:"filename"`
		Category       string `json:"category"`
		VaultType      string `json:"vault_type"`
		Status         string `json:"status"` // INTACT or CORRUPTED
		ExpectedSHA256 string `json:"expected_sha256"`
		ActualSHA256   string `json:"actual_sha256"`
		MerkleRoot     string `json:"merkle_root"`
	}

	results := make([]AuditItem, 0)
	corruptedCount := 0

	dirs := []struct {
		path string
		cat  string
	}{
		{vaultSnapshotsDir, "snapshots"},
		{vaultColdOffloadDir, "cold_offload"},
		{vaultDistilledDir, "distilled"},
	}

	for _, d := range dirs {
		files := listVaultFiles(d.path, d.cat)
		for _, f := range files {
			fullPath := filepath.Join(d.path, f.Filename)
			env, intact, actualHash, err := readAndVerifyEnvelope(fullPath)
			status := "INTACT"
			if err != nil || !intact {
				status = "CORRUPTED"
				corruptedCount++
			}
			results = append(results, AuditItem{
				Filename:       f.Filename,
				Category:       d.cat,
				VaultType:      env.VaultType,
				Status:         status,
				ExpectedSHA256: env.PayloadSHA256,
				ActualSHA256:   actualHash,
				MerkleRoot:     env.MerkleRoot,
			})
		}
	}

	return c.JSON(fiber.Map{
		"success":         true,
		"codename":        "CRYSTAL AUDIT",
		"action":          "INTEGRITY VERIFY",
		"total_scanned":   len(results),
		"corrupted_count": corruptedCount,
		"all_intact":      corruptedCount == 0,
		"results":         results,
		"message":         fmt.Sprintf("🔍 CRYSTAL AUDIT — ตรวจสอบไฟล์สำรองทั้งหมด %d ไฟล์ (เสียหาย/ถูกแก้ไข: %d ไฟล์)", len(results), corruptedCount),
	})
}

// 7. PHOENIX REBOOT — Action: DISASTER RECOVERY
func (h *GodVaultHandler) PhoenixReboot(c *fiber.Ctx) error {
	if !requireGodTherunrock(c) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "message": "เฉพาะ God Therunrock เท่านั้น"})
	}
	var req struct {
		Filename string `json:"filename"`
		Unhide   bool   `json:"unhide"`
	}
	_ = c.BodyParser(&req)

	targetName := strings.TrimSpace(req.Filename)
	if targetName == "" {
		snaps := listVaultFiles(vaultSnapshotsDir, "snapshots")
		if len(snaps) == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"message": "ไม่พบไฟล์ Snapshot ในระบบ กรุณาสร้าง State Snapshot (MIRROR REFLECTION) ก่อน",
			})
		}
		targetName = snaps[0].Filename
	} else {
		targetName = filepath.Base(targetName)
	}

	fullPath := filepath.Join(vaultSnapshotsDir, targetName)
	env, intact, actualHash, err := readAndVerifyEnvelope(fullPath)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "ไม่พบไฟล์ Snapshot: " + err.Error()})
	}
	if !intact {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": fmt.Sprintf("PHOENIX ABORTED: ไฟล์ Snapshot %s ไม่ผ่านการตรวจความสมบูรณ์ (expected %s, got %s)", targetName, env.PayloadSHA256, actualHash),
		})
	}

	var snap StateSnapshotPayload
	if err := json.Unmarshal(env.Payload, &snap); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "Snapshot JSON ไม่ถูกต้อง: " + err.Error()})
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		tables := []interface{}{
			&models.Option{},
			&models.Question{},
			&models.QuizAttempt{},
			&models.Quiz{},
			&models.LessonAssignment{},
			&models.UserLessonProgress{},
			&models.Certificate{},
			&models.Lesson{},
			&models.Course{},
			&models.User{},
		}
		for _, t := range tables {
			if err := tx.Unscoped().Where("1 = 1").Delete(t).Error; err != nil {
				return err
			}
		}

		for _, su := range snap.Users {
			u := su.User
			u.PasswordHash = su.PasswordHash
			u.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&u).Error; err != nil {
				return err
			}
		}
		for _, co := range snap.Courses {
			co.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&co).Error; err != nil {
				return err
			}
		}
		for _, l := range snap.Lessons {
			l.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&l).Error; err != nil {
				return err
			}
		}
		for _, q := range snap.Quizzes {
			q.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&q).Error; err != nil {
				return err
			}
		}
		for _, qu := range snap.Questions {
			qu.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&qu).Error; err != nil {
				return err
			}
		}
		for _, op := range snap.Options {
			op.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&op).Error; err != nil {
				return err
			}
		}
		for _, as := range snap.Assignments {
			as.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&as).Error; err != nil {
				return err
			}
		}
		for _, p := range snap.Progress {
			p.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
		}
		for _, a := range snap.Attempts {
			a.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&a).Error; err != nil {
				return err
			}
		}
		for _, cert := range snap.Certs {
			cert.DeletedAt = gorm.DeletedAt{}
			if err := tx.Create(&cert).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":          true,
		"codename":         "PHOENIX REBOOT",
		"action":           "DISASTER RECOVERY",
		"restored_from":    targetName,
		"restored_users":   len(snap.Users),
		"restored_lessons": len(snap.Lessons),
		"restored_certs":   len(snap.Certs),
		"message":          fmt.Sprintf("🔥 PHOENIX REBOOT — ชุบชีวิตเครือข่ายจาก %s สำเร็จ (%d ผู้ใช้, %d บทเรียน, %d วุฒิบัตร)", targetName, len(snap.Users), len(snap.Lessons), len(snap.Certs)),
	})
}
