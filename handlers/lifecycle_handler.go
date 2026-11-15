package handlers

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"elearning-sbt/database"
	"elearning-sbt/models"

	"github.com/gofiber/fiber/v2"
)

type LifecycleHandler struct{}

func NewLifecycleHandler() *LifecycleHandler {
	return &LifecycleHandler{}
}

// BlockLifecycleInfo describes the current lifecycle stage of a Lesson block
type BlockLifecycleInfo struct {
	Stage        string `json:"stage"`         // GENESIS, FLOURISHING, SUNSETTING, DISSOLVED
	Codename     string `json:"codename"`      // THE SILENT SPARK, PEAK RADIANCE, VELVET TWILIGHT, ASHES TO ETHER
	PeriodLabel  string `json:"period_label"`  // Day 0 – เดือนที่ 2, etc.
	Description  string `json:"description"`   // Poetic Thai description
	AgeDays      int    `json:"age_days"`
	DaysLeft     int    `json:"days_left"`
	IsFrozen     bool   `json:"is_frozen"`     // true for SUNSETTING and DISSOLVED
	IsExpired    bool   `json:"is_expired"`    // true for DISSOLVED (hidden & excluded from stats)
}

// ComputeBlockLifecycle computes the 4-stage lifecycle status from a block's CreatedAt timestamp
func ComputeBlockLifecycle(createdAt time.Time) BlockLifecycleInfo {
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	ageDays := int(time.Since(createdAt).Hours() / 24)
	if ageDays < 0 {
		ageDays = 0
	}
	daysLeft := 730 - ageDays
	if daysLeft < 0 {
		daysLeft = 0
	}

	switch {
	case ageDays <= 60:
		return BlockLifecycleInfo{
			Stage:       "GENESIS",
			Codename:    "THE SILENT SPARK",
			PeriodLabel: "Day 0 – เดือนที่ 2",
			Description: "จุดกำเนิดที่เรียบหรู บล็อกแรกถูกสลักลงระบบอย่างสมบูรณ์แบบ เครือข่ายเริ่มมีชีพจรและเชื่อมต่อโหนดแรกเข้าด้วยกัน",
			AgeDays:     ageDays,
			DaysLeft:    daysLeft,
			IsFrozen:    false,
			IsExpired:   false,
		}
	case ageDays <= 547:
		return BlockLifecycleInfo{
			Stage:       "FLOURISHING",
			Codename:    "PEAK RADIANCE",
			PeriodLabel: "เดือนที่ 3 – 18",
			Description: "ยุครุ่งเรืองเต็มรูปแบบ ระบบเสถียรและทำงานอย่างลื่นไหล เป็นช่วงเวลาทองที่ทุกธุรกรรมและ Smart Contract เปล่งประกายสูงสุด",
			AgeDays:     ageDays,
			DaysLeft:    daysLeft,
			IsFrozen:    false,
			IsExpired:   false,
		}
	case ageDays < 730:
		return BlockLifecycleInfo{
			Stage:       "SUNSETTING",
			Codename:    "VELVET TWILIGHT",
			PeriodLabel: "เดือนที่ 19 – 23",
			Description: "ช่วงนับถอยหลังอันแสนสง่างาม ระบบเริ่มปิดรับธุรกรรมใหม่ (Freeze State) ทิ้งไว้เพียงร่องรอยความทรงจำให้ผู้คนชื่นชมก่อนม่านปิด",
			AgeDays:     ageDays,
			DaysLeft:    daysLeft,
			IsFrozen:    true,
			IsExpired:   false,
		}
	default:
		return BlockLifecycleInfo{
			Stage:       "DISSOLVED",
			Codename:    "ASHES TO ETHER",
			PeriodLabel: "ครบ 2 ปี / Day 730",
			Description: "วินาทีที่ครบกำหนด ระบบรันคำสั่งทำลายตัวเอง (Self-Destruct) สลายคีย์และข้อมูลทั้งหมดหายไปในอากาศอย่างหมดจด เหลือไว้เพียงตำนาน",
			AgeDays:     ageDays,
			DaysLeft:    0,
			IsFrozen:    true,
			IsExpired:   true,
		}
	}
}

type BlockLifecycleItemDTO struct {
	LessonID      uint               `json:"lesson_id"`
	Title         string             `json:"title"`
	CreatedAt     time.Time          `json:"created_at"`
	StudyHours    float64            `json:"study_hours"`
	LearnersCount int64              `json:"learners_count"`
	CertsCount    int64              `json:"certs_count"`
	Lifecycle     BlockLifecycleInfo `json:"lifecycle"`
}

// GetLifecycleOverview returns all blocks with their lifecycle stages & counts (Manager / Godess only)
func (h *LifecycleHandler) GetLifecycleOverview(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	var lessons []models.Lesson
	database.DB.Order("id asc").Find(&lessons)

	items := make([]BlockLifecycleItemDTO, 0, len(lessons))
	stageCounts := map[string]int{
		"GENESIS":     0,
		"FLOURISHING": 0,
		"SUNSETTING":  0,
		"DISSOLVED":   0,
	}

	for _, l := range lessons {
		lc := ComputeBlockLifecycle(l.CreatedAt)
		stageCounts[lc.Stage]++

		var lCount, cCount int64
		database.DB.Model(&models.UserLessonProgress{}).Where("lesson_id = ?", l.ID).Count(&lCount)
		database.DB.Model(&models.Certificate{}).Where("lesson_id = ?", l.ID).Count(&cCount)

		items = append(items, BlockLifecycleItemDTO{
			LessonID:      l.ID,
			Title:         l.Title,
			CreatedAt:     l.CreatedAt,
			StudyHours:    l.StudyHours,
			LearnersCount: lCount,
			CertsCount:    cCount,
			Lifecycle:     lc,
		})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"blocks":       items,
		"stage_counts": stageCounts,
	})
}

// SetBlockAgeDays allows Manager/Godess to adjust a block's CreatedAt for testing/archiving simulation
func (h *LifecycleHandler) SetBlockAgeDays(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	lessonID, err := strconv.ParseUint(c.Params("lessonId"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "Invalid lesson ID"})
	}

	var req struct {
		AgeDays int `json:"age_days"`
	}
	if err := c.BodyParser(&req); err != nil || req.AgeDays < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "กรุณาระบุจำนวนวันให้ถูกต้อง"})
	}

	var lesson models.Lesson
	if err := database.DB.First(&lesson, lessonID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "ไม่พบบล็อกบทเรียน"})
	}

	newCreatedAt := time.Now().Add(-time.Duration(req.AgeDays) * 24 * time.Hour)
	database.DB.Model(&lesson).Update("created_at", newCreatedAt)
	lc := ComputeBlockLifecycle(newCreatedAt)

	return c.JSON(fiber.Map{
		"success":   true,
		"message":   fmt.Sprintf("ปรับอายุบล็อก #%d เป็น %d วัน (%s — %s) แล้ว", lesson.ID, req.AgeDays, lc.Codename, lc.Stage),
		"lifecycle": lc,
	})
}

type ArchivedExpiredBlock struct {
	Lesson       models.Lesson               `json:"lesson"`
	Lifecycle    BlockLifecycleInfo          `json:"lifecycle"`
	ProgressList []models.UserLessonProgress `json:"progress_list"`
	QuizAttempts []models.QuizAttempt        `json:"quiz_attempts"`
	Certificates []models.Certificate        `json:"certificates"`
}

func collectExpiredLessons(lessonIDFilter uint) ([]models.Lesson, error) {
	var allLessons []models.Lesson
	query := database.DB.Preload("Quizzes.Questions.Options").Order("id asc")
	if lessonIDFilter > 0 {
		query = query.Where("id = ?", lessonIDFilter)
	}
	if err := query.Find(&allLessons).Error; err != nil {
		return nil, err
	}

	var expired []models.Lesson
	for _, l := range allLessons {
		if ComputeBlockLifecycle(l.CreatedAt).IsExpired {
			expired = append(expired, l)
		}
	}
	return expired, nil
}

// ExportExpiredBlocks downloads all DISSOLVED (expired) blocks as CSV (UTF-8 BOM) or JSON
func (h *LifecycleHandler) ExportExpiredBlocks(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	format := c.Query("format", "csv")
	scope := c.Query("scope", "expired")
	lessonIDFilter, _ := strconv.ParseUint(c.Query("lesson_id", "0"), 10, 32)

	var expiredLessons []models.Lesson
	var err error
	if scope == "all" {
		query := database.DB.Preload("Quizzes.Questions.Options").Order("id asc")
		if lessonIDFilter > 0 {
			query = query.Where("id = ?", lessonIDFilter)
		}
		err = query.Find(&expiredLessons).Error
	} else {
		expiredLessons, err = collectExpiredLessons(uint(lessonIDFilter))
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	if len(expiredLessons) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ยังไม่มีบล็อกที่หมดอายุ (DISSOLVED / ครบ 2 ปี) สำหรับส่งออก",
		})
	}

	dateStr := time.Now().Format("2006-01-02")

	if format == "json" {
		archives := make([]ArchivedExpiredBlock, 0, len(expiredLessons))
		for _, l := range expiredLessons {
			var progs []models.UserLessonProgress
			database.DB.Where("lesson_id = ?", l.ID).Find(&progs)

			var attempts []models.QuizAttempt
			database.DB.Table("quiz_attempts").
				Joins("JOIN quizzes ON quizzes.id = quiz_attempts.quiz_id").
				Where("quizzes.lesson_id = ?", l.ID).
				Find(&attempts)

			var certs []models.Certificate
			database.DB.Where("lesson_id = ?", l.ID).Find(&certs)

			archives = append(archives, ArchivedExpiredBlock{
				Lesson:       l,
				Lifecycle:    ComputeBlockLifecycle(l.CreatedAt),
				ProgressList: progs,
				QuizAttempts: attempts,
				Certificates: certs,
			})
		}

		payload := fiber.Map{
			"archive_type":   "LearnChain_Expired_Blocks_Archive",
			"exported_at":    time.Now(),
			"expired_count":  len(archives),
			"expired_blocks": archives,
		}
		jsonBytes, _ := json.MarshalIndent(payload, "", "  ")
		c.Set("Content-Type", "application/json; charset=utf-8")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=Expired_Blocks_Archive_%s.json", dateStr))
		return c.Send(jsonBytes)
	}

	// Default: CSV with UTF-8 BOM for clean Thai display in Microsoft Excel
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{
		"LessonID", "LessonTitle", "LifecycleStage", "Codename", "AgeDays", "CreatedAt",
		"StudyHours", "LearnerName", "LearnerEmail", "Ward",
		"PreTestPassed", "WatchedSec", "PostTestPassed", "IsCompleted", "OnChainTxHash",
		"CertificateTokenID", "CertificateScorePct", "CertificateIssuedAt",
	})

	userMap := map[uint]models.User{}
	var allUsers []models.User
	database.DB.Find(&allUsers)
	for _, u := range allUsers {
		userMap[u.ID] = u
	}

	for _, l := range expiredLessons {
		lc := ComputeBlockLifecycle(l.CreatedAt)
		var progs []models.UserLessonProgress
		database.DB.Where("lesson_id = ?", l.ID).Find(&progs)

		var certs []models.Certificate
		database.DB.Where("lesson_id = ?", l.ID).Find(&certs)
		certByUser := map[uint]models.Certificate{}
		for _, cert := range certs {
			certByUser[cert.UserID] = cert
		}

		if len(progs) == 0 {
			_ = w.Write([]string{
				strconv.FormatUint(uint64(l.ID), 10), l.Title, lc.Stage, lc.Codename,
				strconv.Itoa(lc.AgeDays), l.CreatedAt.Format("2006-01-02"),
				fmt.Sprintf("%.1f", l.StudyHours), "-", "-", "-", "-", "0", "-", "-", "-", "-", "-", "-",
			})
			continue
		}

		for _, p := range progs {
			u := userMap[p.UserID]
			cert := certByUser[p.UserID]
			certIssued := ""
			if !cert.IssuedAt.IsZero() {
				certIssued = cert.IssuedAt.Format("2006-01-02")
			}
			_ = w.Write([]string{
				strconv.FormatUint(uint64(l.ID), 10),
				l.Title,
				lc.Stage,
				lc.Codename,
				strconv.Itoa(lc.AgeDays),
				l.CreatedAt.Format("2006-01-02"),
				fmt.Sprintf("%.1f", l.StudyHours),
				u.Name,
				u.Email,
				u.Ward,
				strconv.FormatBool(p.PreTestPassed),
				fmt.Sprintf("%.0f", p.LastWatchedSec),
				strconv.FormatBool(p.PostTestPassed),
				strconv.FormatBool(p.IsCompleted),
				p.OnChainTxHash,
				cert.TokenID,
				fmt.Sprintf("%.1f", cert.AverageScore),
				certIssued,
			})
		}
	}
	w.Flush()

	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=Expired_Blocks_Archive_%s.csv", dateStr))
	return c.Send(buf.Bytes())
}

// PurgeExpiredBlocks executes Self-Destruct (permanent hard-delete) on DISSOLVED blocks
func (h *LifecycleHandler) PurgeExpiredBlocks(c *fiber.Ctx) error {
	if _, ok := requireManagerOrSecretAdmin(c); !ok {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้จัดการเท่านั้น",
		})
	}

	lessonIDFilter, _ := strconv.ParseUint(c.Query("lesson_id", "0"), 10, 32)
	expiredLessons, err := collectExpiredLessons(uint(lessonIDFilter))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	if len(expiredLessons) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบบล็อกที่หมดอายุ (DISSOLVED) สำหรับทำลายถาวร",
		})
	}

	purgedCount := 0
	for _, l := range expiredLessons {
		// Delete quizzes -> questions -> options & attempts
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
		purgedCount++
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"purged_count": purgedCount,
		"message":      fmt.Sprintf("💨 ASHES TO ETHER — ทำลายบล็อกที่หมดอายุถาวรจำนวน %d บล็อกเรียบร้อยแล้ว", purgedCount),
	})
}
