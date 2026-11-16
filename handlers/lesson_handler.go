package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elearning-sbt/database"
	"elearning-sbt/middleware"
	"elearning-sbt/models"
	"elearning-sbt/services"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LessonHandler struct {
	courseService     *services.CourseService
	blockchainService *services.BlockchainService
}

func NewLessonHandler(cs *services.CourseService, bs *services.BlockchainService) *LessonHandler {
	return &LessonHandler{courseService: cs, blockchainService: bs}
}

const coversDir = "./covers"

// RemoveCoverIfUnused deletes a custom /covers/cover_xxx file from disk if no active lesson references it.
func RemoveCoverIfUnused(thumbnailURL string) {
	if !strings.HasPrefix(thumbnailURL, "/covers/") {
		return
	}
	filename := filepath.Base(strings.TrimPrefix(thumbnailURL, "/covers/"))
	if filename == "" || filename == "." || filename == ".." {
		return
	}
	var refCount int64
	database.DB.Model(&models.Lesson{}).Where("thumbnail_url = ?", thumbnailURL).Count(&refCount)
	if refCount == 0 {
		_ = os.Remove(filepath.Join(coversDir, filename))
	}
}

// UploadCover handles uploading a lesson cover image into ./covers/cover_xxx.jpg
func (h *LessonHandler) UploadCover(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)
	var user models.User
	if userID == SecretAdminUserID {
		user = BuildSecretAdminUser("manager")
	} else if err := database.DB.First(&user, userID).Error; err != nil || (user.Role != "creator" && user.Role != "manager") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้สร้างบทเรียนหรือผู้จัดการเท่านั้นที่สามารถอัปโหลดภาพปกได้",
		})
	}

	file, err := c.FormFile("cover")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "กรุณาเลือกไฟล์ภาพหน้าปก",
		})
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
	default:
		ct := strings.ToLower(file.Header.Get("Content-Type"))
		if strings.HasPrefix(ct, "image/") {
			ext = ".jpg"
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"message": "รองรับเฉพาะไฟล์รูปภาพ (.jpg, .png, .webp, .gif)",
			})
		}
	}

	if err := os.MkdirAll(coversDir, 0755); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถสร้างโฟลเดอร์เก็บรูปภาพได้",
		})
	}

	filename := fmt.Sprintf("cover_%d%s", time.Now().UnixNano()/1e6, ext)
	savePath := filepath.Join(coversDir, filename)
	if err := c.SaveFile(file, savePath); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถบันทึกไฟล์ภาพหน้าปกได้",
		})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"filename": filename,
		"url":      "/covers/" + filename,
	})
}

// GetLesson returns lesson material, quizzes metadata, and learner's progress
func (h *LessonHandler) GetLesson(c *fiber.Ctx) error {
	lessonIDStr := c.Params("lessonId")
	lessonID, err := strconv.ParseUint(lessonIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid lesson ID",
		})
	}

	var lesson models.Lesson
	err = database.DB.Preload("Quizzes").First(&lesson, lessonID).Error
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Lesson not found",
		})
	}

	userID := middleware.ResolveUserID(c)
	progress, err := h.courseService.GetOrCreateProgress(userID, uint(lessonID))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "เกิดข้อผิดพลาดในการโหลดความคืบหน้า: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"lesson":   lesson,
		"progress": progress,
	})
}

// UpdateProgress receives playback heartbeats and executes anti-skip checks
func (h *LessonHandler) UpdateProgress(c *fiber.Ctx) error {
	lessonIDStr := c.Params("lessonId")
	lessonID, err := strconv.ParseUint(lessonIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid lesson ID",
		})
	}

	var req models.VideoProgressRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
		})
	}

	if req.PlaybackRate <= 0 {
		req.PlaybackRate = 1.0
	}

	userID := middleware.ResolveUserID(c)

	var user models.User
	if userID == SecretAdminUserID {
		user = BuildSecretAdminUser("creator")
	} else {
		database.DB.First(&user, userID)
	}
	// ถ้าเป็นผู้สร้าง (Creator) หรือผู้ดูแลระบบลับ: โหมดทดสอบระบบ ไม่เก็บสถิติและไม่บันทึกลงบล็อกเชน
	if user.Role == "creator" {
		return c.JSON(fiber.Map{
			"success":          true,
			"message":          "โหมดทดสอบสำหรับผู้สร้าง (ไม่เก็บสถิติ)",
			"is_creator_test":  true,
			"progress": models.UserLessonProgress{
				UserID:         userID,
				LessonID:       uint(lessonID),
				VideoWatched:   true,
				LastWatchedSec: req.CurrentSec,
			},
		})
	}

	var targetLesson models.Lesson
	if err := database.DB.First(&targetLesson, lessonID).Error; err == nil {
		lc := ComputeBlockLifecycle(targetLesson.CreatedAt)
		if lc.IsFrozen {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("⏳ บล็อกนี้อยู่ในสถานะ %s — %s (%s) ปิดรับธุรกรรมใหม่แล้ว", lc.Codename, lc.Stage, lc.PeriodLabel),
			})
		}
	}

	updatedProgress, err := h.courseService.UpdateVideoProgress(userID, uint(lessonID), req.CurrentSec, req.PlaybackRate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success":  false,
			"message":  err.Error(),
			"progress": updatedProgress,
		})
	}

	if updatedProgress.VideoWatched && updatedProgress.OnChainTxHash == "" {
		var lesson models.Lesson
		database.DB.First(&lesson, lessonID)
		txHash, _ := h.blockchainService.RecordAttendanceOnChain(user.WalletAddress, lesson.CourseID, uint(lessonID), updatedProgress.LastWatchedSec)
		updatedProgress.OnChainTxHash = txHash
		database.DB.Save(updatedProgress)
	}

	if updatedProgress.IsCompleted {
		_, _ = h.courseService.EnsureLessonCertificate(userID, uint(lessonID), h.blockchainService)
	}

	return c.JSON(fiber.Map{
		"success":            true,
		"message":            "Playback progress updated",
		"progress":           updatedProgress,
		"on_chain_tx_hash":   updatedProgress.OnChainTxHash,
	})
}

// CreateLesson allows a creator (or secret admin) to insert/create a new lesson along with its quizzes
func (h *LessonHandler) CreateLesson(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)
	var currentUser models.User
	if userID == SecretAdminUserID {
		currentUser = BuildSecretAdminUser("creator")
	} else if err := database.DB.First(&currentUser, userID).Error; err != nil || (currentUser.Role != "creator" && currentUser.Role != "manager") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้สร้างเนื้อหา (Creator) เท่านั้นที่สามารถแทรกบทเรียนได้",
		})
	}

	var req models.CreateLessonRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลแบบฟอร์มไม่ถูกต้อง",
			"error":   err.Error(),
		})
	}

	if strings.TrimSpace(req.Title) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "กรุณาระบุชื่อเรื่องบทเรียน",
		})
	}

	courseID := req.CourseID
	if courseID == 0 {
		var userCourse models.Course
		if err := database.DB.Where("creator_id = ?", userID).First(&userCourse).Error; err == nil {
			courseID = userCourse.ID
		} else {
			courseID = 1
		}
	}

	var course models.Course
	if err := database.DB.First(&course, courseID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบคอร์สหลักสูตรในระบบ",
		})
	}

	// Update creator name if provided
	if req.CreatorName != "" {
		var creator models.User
		if err := database.DB.First(&creator, course.CreatorID).Error; err == nil {
			creator.Name = strings.TrimSpace(req.CreatorName)
			database.DB.Save(&creator)
		}
	}

	// Calculate SortOrder if not specified or <= 0
	if req.SortOrder <= 0 {
		var maxSort int
		database.DB.Model(&models.Lesson{}).Where("course_id = ?", courseID).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxSort)
		req.SortOrder = maxSort + 1
	} else {
		// If inserting at a specific sort_order, shift existing lessons with sort_order >= req.SortOrder
		database.DB.Model(&models.Lesson{}).
			Where("course_id = ? AND sort_order >= ?", courseID, req.SortOrder).
			Update("sort_order", gorm.Expr("sort_order + 1"))
	}

	if req.DurationSec <= 0 {
		req.DurationSec = 60 // default 60 sec
	}
	if req.StudyHours <= 0 {
		req.StudyHours = 1 // default 1 hour
	}

	openPeriod := strings.TrimSpace(req.OpenPeriod)
	if openPeriod == "" {
		openPeriod = "เปิดตลอดเวลา"
	}

	passScorePct := req.PassScorePct
	if passScorePct <= 0 {
		passScorePct = 70
	}
	if passScorePct > 100 {
		passScorePct = 100
	}

	lesson := models.Lesson{
		CourseID:     courseID,
		Title:        strings.TrimSpace(req.Title),
		Summary:      strings.TrimSpace(req.Summary),
		Description:  strings.TrimSpace(req.Description),
		ThumbnailURL: strings.TrimSpace(req.ThumbnailURL),
		YoutubeURL:   strings.TrimSpace(req.YoutubeURL),
		DocumentURL:  strings.TrimSpace(req.DocumentURL),
		DurationSec:  req.DurationSec,
		SignatoryIDs: strings.TrimSpace(req.SignatoryIDs),
		StudyHours:   req.StudyHours,
		SortOrder:    req.SortOrder,
		IsHidden:     req.IsHidden,
		OpenPeriod:      openPeriod,
		PassScorePct:    passScorePct,
		DisablePreTest:  req.DisablePreTest,
		DisablePostTest: req.DisablePostTest,
	}

	if err := database.DB.Create(&lesson).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถสร้างบทเรียนได้",
			"error":   err.Error(),
		})
	}

	// Create Quizzes (Pre-test and Post-test)
	for _, qDTO := range req.Quizzes {
		quizType := qDTO.Type
		if quizType == "" {
			quizType = "lesson_pre_test"
		}
		quizTitle := qDTO.Title
		if quizTitle == "" {
			if quizType == "lesson_pre_test" {
				quizTitle = lesson.Title + ": แบบทดสอบก่อนเรียน (Pre-Test)"
			} else {
				quizTitle = lesson.Title + ": แบบทดสอบหลังเรียน (Post-Test)"
			}
		}

		quiz := models.Quiz{
			LessonID: lesson.ID,
			Title:    quizTitle,
			Type:     quizType,
		}
		if err := database.DB.Create(&quiz).Error; err != nil {
			continue
		}

		for _, questionDTO := range qDTO.Questions {
			if strings.TrimSpace(questionDTO.QuestionText) == "" {
				continue
			}
			question := models.Question{
				QuizID:       quiz.ID,
				QuestionText: strings.TrimSpace(questionDTO.QuestionText),
			}
			if err := database.DB.Create(&question).Error; err != nil {
				continue
			}

			for _, optDTO := range questionDTO.Options {
				if strings.TrimSpace(optDTO.OptionText) == "" {
					continue
				}
				option := models.Option{
					QuestionID: question.ID,
					OptionText: strings.TrimSpace(optDTO.OptionText),
					IsCorrect:  optDTO.IsCorrect,
				}
				database.DB.Create(&option)
			}
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "แทรกบทเรียนใหม่เข้าสู่ระบบเรียบร้อยแล้ว",
		"lesson":  lesson,
	})
}

// DeleteLesson removes a lesson (allowed for Creator, Manager, and secret admin)
func (h *LessonHandler) DeleteLesson(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)
	var user models.User
	if userID == SecretAdminUserID {
		user = BuildSecretAdminUser("manager")
	} else if err := database.DB.First(&user, userID).Error; err != nil || (user.Role != "creator" && user.Role != "manager") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"message": "สิทธิ์การใช้งานถูกปฏิเสธ: เฉพาะผู้สร้างบทเรียนหรือผู้จัดการเท่านั้นที่สามารถลบบทเรียนได้",
		})
	}

	lessonIDStr := c.Params("lessonId")
	lessonID, err := strconv.ParseUint(lessonIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid lesson ID",
		})
	}

	var lesson models.Lesson
	if err := database.DB.First(&lesson, lessonID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Lesson not found",
		})
	}

	deletedThumb := lesson.ThumbnailURL
	database.DB.Where("lesson_id = ?", lesson.ID).Delete(&models.LessonAssignment{})
	database.DB.Where("lesson_id = ?", lesson.ID).Delete(&models.UserLessonProgress{})
	database.DB.Delete(&lesson)
	RemoveCoverIfUnused(deletedThumb)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "ลบบทเรียนเรียบร้อยแล้ว",
	})
}

// UpdateLesson updates a lesson's metadata
func (h *LessonHandler) UpdateLesson(c *fiber.Ctx) error {
	lessonIDStr := c.Params("lessonId")
	lessonID, err := strconv.ParseUint(lessonIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid lesson ID",
		})
	}

	var req models.UpdateLessonRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
		})
	}

	var lesson models.Lesson
	if err := database.DB.First(&lesson, lessonID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบบทเรียนที่ต้องการแก้ไข",
		})
	}

	oldThumb := lesson.ThumbnailURL
	if strings.TrimSpace(req.Title) != "" {
		lesson.Title = strings.TrimSpace(req.Title)
	}
	if strings.TrimSpace(req.Summary) != "" {
		lesson.Summary = strings.TrimSpace(req.Summary)
	}
	if strings.TrimSpace(req.Description) != "" {
		lesson.Description = strings.TrimSpace(req.Description)
	}
	if strings.TrimSpace(req.ThumbnailURL) != "" {
		lesson.ThumbnailURL = strings.TrimSpace(req.ThumbnailURL)
	}
	if strings.TrimSpace(req.YoutubeURL) != "" {
		lesson.YoutubeURL = strings.TrimSpace(req.YoutubeURL)
	}
	if strings.TrimSpace(req.DocumentURL) != "" {
		lesson.DocumentURL = strings.TrimSpace(req.DocumentURL)
	}
	if req.DurationSec > 0 {
		lesson.DurationSec = req.DurationSec
	}
	if req.StudyHours > 0 {
		lesson.StudyHours = req.StudyHours
	}
	if req.SortOrder > 0 {
		lesson.SortOrder = req.SortOrder
	}
	if req.IsHidden != nil {
		lesson.IsHidden = *req.IsHidden
	}
	if strings.TrimSpace(req.OpenPeriod) != "" {
		lesson.OpenPeriod = strings.TrimSpace(req.OpenPeriod)
	}
	if req.PassScorePct > 0 {
		if req.PassScorePct > 100 {
			lesson.PassScorePct = 100
		} else {
			lesson.PassScorePct = req.PassScorePct
		}
	}
	if req.SignatoryIDs != nil {
		lesson.SignatoryIDs = strings.TrimSpace(*req.SignatoryIDs)
	}
	if req.DisablePreTest != nil {
		lesson.DisablePreTest = *req.DisablePreTest
		if !*req.DisablePreTest {
			database.DB.Unscoped().Model(&models.Quiz{}).Where("lesson_id = ? AND type = ?", lesson.ID, "lesson_pre_test").Update("deleted_at", nil)
		}
	}
	if req.DisablePostTest != nil {
		lesson.DisablePostTest = *req.DisablePostTest
		if !*req.DisablePostTest {
			database.DB.Unscoped().Model(&models.Quiz{}).Where("lesson_id = ? AND type = ?", lesson.ID, "lesson_post_test").Update("deleted_at", nil)
		}
	}

	if err := database.DB.Save(&lesson).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถบันทึกการแก้ไขบทเรียนได้",
		})
	}
	if oldThumb != "" && oldThumb != lesson.ThumbnailURL {
		RemoveCoverIfUnused(oldThumb)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "บันทึกการแก้ไขบทเรียนสำเร็จแล้ว",
		"lesson":  lesson,
	})
}

// ToggleHideLesson toggles lesson visibility
func (h *LessonHandler) ToggleHideLesson(c *fiber.Ctx) error {
	lessonIDStr := c.Params("lessonId")
	lessonID, err := strconv.ParseUint(lessonIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid lesson ID",
		})
	}

	var lesson models.Lesson
	if err := database.DB.First(&lesson, lessonID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Lesson not found",
		})
	}

	lesson.IsHidden = !lesson.IsHidden
	database.DB.Save(&lesson)

	statusText := "แสดงบทเรียนแล้ว"
	if lesson.IsHidden {
		statusText = "ซ่อนบทเรียนจากผู้เรียนแล้ว"
	}

	return c.JSON(fiber.Map{
		"success":   true,
		"message":   statusText,
		"is_hidden": lesson.IsHidden,
		"lesson":    lesson,
	})
}

// UpdateLessonSchedule updates available dates/times for a lesson
func (h *LessonHandler) UpdateLessonSchedule(c *fiber.Ctx) error {
	lessonIDStr := c.Params("lessonId")
	lessonID, err := strconv.ParseUint(lessonIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid lesson ID",
		})
	}

	var req struct {
		OpenPeriod string `json:"open_period"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
		})
	}

	var lesson models.Lesson
	if err := database.DB.First(&lesson, lessonID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Lesson not found",
		})
	}

	lesson.OpenPeriod = strings.TrimSpace(req.OpenPeriod)
	database.DB.Save(&lesson)

	return c.JSON(fiber.Map{
		"success":     true,
		"message":     "อัปเดตระยะเวลาเปิดให้เรียนเรียบร้อยแล้ว",
		"open_period": lesson.OpenPeriod,
		"lesson":      lesson,
	})
}

// ComputeLessonRatingStats aggregates 1-5 star ratings for a lesson without exposing voter identities
func ComputeLessonRatingStats(lessonID uint, callerUserID uint) models.LessonRatingStatsDTO {
	var ratings []models.LessonRating
	database.DB.Where("lesson_id = ?", lessonID).Order("updated_at desc").Find(&ratings)

	counts := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	var sum int
	myRating := 0
	myComment := ""
	var comments []models.LessonRatingCommentDTO
	for _, r := range ratings {
		if r.Rating >= 1 && r.Rating <= 5 {
			counts[r.Rating]++
			sum += r.Rating
		}
		if callerUserID > 0 && r.UserID == callerUserID {
			myRating = r.Rating
			myComment = r.Comment
		}
		if strings.TrimSpace(r.Comment) != "" {
			comments = append(comments, models.LessonRatingCommentDTO{
				ID:        r.ID,
				LessonID:  r.LessonID,
				Rating:    r.Rating,
				Comment:   strings.TrimSpace(r.Comment),
				CreatedAt: r.UpdatedAt,
			})
		}
	}

	total := int64(len(ratings))
	avg := 0.0
	if total > 0 {
		avg = float64(sum) / float64(total)
	}

	return models.LessonRatingStatsDTO{
		LessonID:      lessonID,
		AverageRating: avg,
		TotalRatings:  total,
		StarCounts:    counts,
		MyRating:      myRating,
		MyComment:     myComment,
		Comments:      comments,
	}
}

// RateLesson lets a learner submit or update their 1-5 star rating and optional comment for a lesson
func (h *LessonHandler) RateLesson(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)
	lessonID, err := strconv.ParseUint(c.Params("lessonId"), 10, 32)
	if err != nil || lessonID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "Invalid lesson ID"})
	}

	var req struct {
		Rating  int     `json:"rating"`
		Comment *string `json:"comment"`
	}
	if err := c.BodyParser(&req); err != nil || req.Rating < 1 || req.Rating > 5 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "กรุณาให้คะแนนระหว่าง 1 ถึง 5 ดาว",
		})
	}

	var lesson models.Lesson
	if err := database.DB.First(&lesson, lessonID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "ไม่พบบทเรียน"})
	}

	var existing models.LessonRating
	err = database.DB.Where("lesson_id = ? AND user_id = ?", uint(lessonID), userID).First(&existing).Error
	commentVal := ""
	if err == nil {
		commentVal = existing.Comment
	}
	if req.Comment != nil {
		commentVal = strings.TrimSpace(*req.Comment)
	}

	record := models.LessonRating{
		LessonID: uint(lessonID),
		UserID:   userID,
		Rating:   req.Rating,
		Comment:  commentVal,
	}
	err = database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "lesson_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"rating", "comment", "updated_at"}),
	}).Create(&record).Error
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	stats := ComputeLessonRatingStats(uint(lessonID), userID)
	return c.JSON(fiber.Map{
		"success":    true,
		"message":    fmt.Sprintf("⭐ บันทึกคะแนน %d ดาวเรียบร้อยแล้ว", req.Rating),
		"my_rating":  req.Rating,
		"my_comment": commentVal,
		"stats":      stats,
	})
}

// GetLessonRatings returns anonymous aggregated 1-5 star stats for a lesson
func (h *LessonHandler) GetLessonRatings(c *fiber.Ctx) error {
	userID := middleware.ResolveUserID(c)
	lessonID, err := strconv.ParseUint(c.Params("lessonId"), 10, 32)
	if err != nil || lessonID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "Invalid lesson ID"})
	}

	var lesson models.Lesson
	if err := database.DB.First(&lesson, lessonID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "message": "ไม่พบบทเรียน"})
	}

	stats := ComputeLessonRatingStats(uint(lessonID), userID)
	return c.JSON(fiber.Map{
		"success":      true,
		"lesson_id":    lesson.ID,
		"lesson_title": lesson.Title,
		"stats":        stats,
	})
}

// GetCreatorCertificateData returns per-lesson learner counts, average star ratings,
// and anonymous learner comments filtered by optional start_date & end_date (YYYY-MM-DD).
func (h *LessonHandler) GetCreatorCertificateData(c *fiber.Ctx) error {
	startStr := strings.TrimSpace(c.Query("start_date"))
	endStr := strings.TrimSpace(c.Query("end_date"))

	var startTime, endTime *time.Time
	if startStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", startStr, time.Local); err == nil {
			startTime = &t
		}
	}
	if endStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", endStr, time.Local); err == nil {
			eod := t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			endTime = &eod
		}
	}

	var lessons []models.Lesson
	database.DB.Order("sort_order asc, id asc").Find(&lessons)

	type lessonStatRow struct {
		LessonID      uint    `json:"lesson_id"`
		Title         string  `json:"title"`
		StudyHours    float64 `json:"study_hours"`
		LearnerCount  int64   `json:"learner_count"`
		AverageRating float64 `json:"average_rating"`
		TotalRatings  int64   `json:"total_ratings"`
	}
	type commentRow struct {
		ID          uint   `json:"id"`
		LessonID    uint   `json:"lesson_id"`
		LessonTitle string `json:"lesson_title"`
		Rating      int    `json:"rating"`
		Comment     string `json:"comment"`
		DateText    string `json:"date_text"`
	}

	var rows []lessonStatRow
	var allComments []commentRow
	var totalLearners int64
	var ratingSum float64
	var ratingCount int64

	for _, l := range lessons {
		pQuery := database.DB.Model(&models.UserLessonProgress{}).Where("lesson_id = ?", l.ID)
		if startTime != nil {
			pQuery = pQuery.Where("updated_at >= ?", *startTime)
		}
		if endTime != nil {
			pQuery = pQuery.Where("updated_at <= ?", *endTime)
		}
		var lCount int64
		pQuery.Distinct("user_id").Count(&lCount)
		totalLearners += lCount

		rQuery := database.DB.Where("lesson_id = ?", l.ID)
		if startTime != nil {
			rQuery = rQuery.Where("updated_at >= ?", *startTime)
		}
		if endTime != nil {
			rQuery = rQuery.Where("updated_at <= ?", *endTime)
		}
		var ratings []models.LessonRating
		rQuery.Order("updated_at desc").Find(&ratings)

		var lSum int
		for _, r := range ratings {
			lSum += r.Rating
			if strings.TrimSpace(r.Comment) != "" {
				allComments = append(allComments, commentRow{
					ID:          r.ID,
					LessonID:    l.ID,
					LessonTitle: l.Title,
					Rating:      r.Rating,
					Comment:     strings.TrimSpace(r.Comment),
					DateText:    r.UpdatedAt.Format("02/01/2006"),
				})
			}
		}
		avg := 0.0
		if len(ratings) > 0 {
			avg = float64(lSum) / float64(len(ratings))
			ratingSum += float64(lSum)
			ratingCount += int64(len(ratings))
		}

		rows = append(rows, lessonStatRow{
			LessonID:      l.ID,
			Title:         l.Title,
			StudyHours:    l.StudyHours,
			LearnerCount:  lCount,
			AverageRating: avg,
			TotalRatings:  int64(len(ratings)),
		})
	}

	overallAvg := 0.0
	if ratingCount > 0 {
		overallAvg = ratingSum / float64(ratingCount)
	}

	return c.JSON(fiber.Map{
		"success":        true,
		"lessons":        rows,
		"comments":       allComments,
		"total_lessons":  len(rows),
		"total_learners": totalLearners,
		"overall_rating": overallAvg,
		"total_ratings":  ratingCount,
	})
}
