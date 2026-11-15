package handlers

import (
	"strconv"
	"strings"

	"elearning-sbt/database"
	"elearning-sbt/middleware"
	"elearning-sbt/models"
	"elearning-sbt/services"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type CourseHandler struct {
	courseService *services.CourseService
}

func NewCourseHandler(cs *services.CourseService) *CourseHandler {
	return &CourseHandler{courseService: cs}
}

// ListCourses returns all published courses, optionally filtered by creator_id
func (h *CourseHandler) ListCourses(c *fiber.Ctx) error {
	creatorIDStr := c.Query("creator_id")
	query := database.DB.Preload("Creator")
	if creatorIDStr != "" {
		if creatorID, err := strconv.ParseUint(creatorIDStr, 10, 32); err == nil {
			query = query.Where("creator_id = ?", creatorID)
		}
	}

	var courses []models.Course
	query.Order("id asc").Find(&courses)

	return c.JSON(fiber.Map{
		"success": true,
		"courses": courses,
	})
}

// GetCourseByCreator returns or auto-creates the course for a specific creator
func (h *CourseHandler) GetCourseByCreator(c *fiber.Ctx) error {
	creatorIDStr := c.Params("creatorId")
	creatorID, err := strconv.ParseUint(creatorIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid creator ID",
		})
	}

	var course models.Course
	err = database.DB.Preload("Creator").
		Preload("Lessons", func(db *gorm.DB) *gorm.DB {
			return db.Order("lessons.sort_order ASC")
		}).
		Preload("Lessons.Quizzes").
		Where("creator_id = ?", creatorID).First(&course).Error

	if err != nil {
		// If no course exists for this creator yet, auto-create one
		var creator models.User
		if errUser := database.DB.First(&creator, creatorID).Error; errUser != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"message": "Creator not found",
			})
		}

		course = models.Course{
			Title:        "หลักสูตรของ " + creator.Name,
			Summary:      "ยินดีต้อนรับสู่หลักสูตรใหม่ จัดทำโดย " + creator.Name + " สามารถคลิกแก้ไขข้อมูลเพื่อปรับปรุงเนื้อหาได้",
			Description:  "รายละเอียดหลักสูตรการเรียนรู้",
			ThumbnailURL: "https://images.unsplash.com/photo-1516321318423-f06f85e504b3?q=80&w=800&auto=format&fit=crop",
			CreatorID:    creator.ID,
		}
		if errCreate := database.DB.Create(&course).Error; errCreate != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"message": "Failed to create course for creator",
			})
		}
		database.DB.Preload("Creator").First(&course, course.ID)
	}

	for i := range course.Lessons {
		course.Lessons[i].RatingStats = ComputeLessonRatingStats(course.Lessons[i].ID, 0)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"course":  course,
	})
}

// UpdateCourse allows a creator to update their course info (Title, Summary, Cover, Creator Name)
func (h *CourseHandler) UpdateCourse(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid course ID",
		})
	}

	var req models.UpdateCourseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "ข้อมูลไม่ถูกต้อง",
		})
	}

	var course models.Course
	if err := database.DB.First(&course, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "ไม่พบหลักสูตรที่ต้องการแก้ไข",
		})
	}

	if strings.TrimSpace(req.Title) != "" {
		course.Title = strings.TrimSpace(req.Title)
	}
	if strings.TrimSpace(req.Summary) != "" {
		course.Summary = strings.TrimSpace(req.Summary)
		course.Description = strings.TrimSpace(req.Summary)
	}
	if strings.TrimSpace(req.ThumbnailURL) != "" {
		course.ThumbnailURL = strings.TrimSpace(req.ThumbnailURL)
	}
	if err := database.DB.Save(&course).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"message": "ไม่สามารถบันทึกข้อมูลหลักสูตรได้",
		})
	}

	// Update creator name if provided
	if strings.TrimSpace(req.CreatorName) != "" {
		var creator models.User
		if err := database.DB.First(&creator, course.CreatorID).Error; err == nil {
			creator.Name = strings.TrimSpace(req.CreatorName)
			database.DB.Save(&creator)
		}
	}

	// Reload with Creator
	database.DB.Preload("Creator").First(&course, course.ID)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "อัปเดตข้อมูลหลักสูตรสำเร็จแล้ว",
		"course":  course,
	})
}

// CourseProgressLessonDTO represents a lesson decorated with learner-specific status
type CourseProgressLessonDTO struct {
	models.Lesson
	IsLocked    bool                        `json:"is_locked"`
	IsAssigned  bool                        `json:"is_assigned"`
	Lifecycle   BlockLifecycleInfo          `json:"lifecycle"`
	RatingStats models.LessonRatingStatsDTO `json:"rating_stats"`
	Progress    *models.UserLessonProgress  `json:"progress"`
	Certificate *models.Certificate         `json:"certificate,omitempty"`
}

// GetCourseDetails returns course with lessons, quizzes, and learner's progress
func (h *CourseHandler) GetCourseDetails(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid course ID",
		})
	}

	var course models.Course
	err = database.DB.Preload("Creator").
		Preload("Lessons", func(db *gorm.DB) *gorm.DB {
			return db.Order("lessons.sort_order ASC")
		}).
		Preload("Lessons.Quizzes").
		First(&course, id).Error
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Course not found",
		})
	}

	// Resolve user ID
	userID := middleware.ResolveUserID(c)

	// Fetch assigned lessons for this user
	var assignments []models.LessonAssignment
	database.DB.Where("user_id = ?", userID).Find(&assignments)
	assignedLessonMap := make(map[uint]bool)
	for _, a := range assignments {
		assignedLessonMap[a.LessonID] = true
	}

	// Fetch all lessons across the platform so lessons created by any creator appear in Learner search & assignments
	var allLessons []models.Lesson
	database.DB.Preload("Quizzes").Order("sort_order asc, id asc").Find(&allLessons)

	// Fetch progress for each lesson & determine lock status (skipping DISSOLVED / expired blocks)
	decoratedLessons := make([]CourseProgressLessonDTO, 0, len(allLessons))
	for _, l := range allLessons {
		lc := ComputeBlockLifecycle(l.CreatedAt)
		if lc.IsExpired {
			continue
		}
		progress, _ := h.courseService.GetOrCreateProgress(userID, l.ID)
		allowed, _, _ := h.courseService.CheckLessonAccess(userID, l.ID)

		var lessonCert *models.Certificate
		if progress != nil && progress.IsCompleted {
			lessonCert, _ = h.courseService.EnsureLessonCertificate(userID, l.ID, nil)
		}

		decoratedLessons = append(decoratedLessons, CourseProgressLessonDTO{
			Lesson:      l,
			IsLocked:    !allowed,
			IsAssigned:  assignedLessonMap[l.ID],
			Lifecycle:   lc,
			RatingStats: ComputeLessonRatingStats(l.ID, userID),
			Progress:    progress,
			Certificate: lessonCert,
		})
	}

	// Check overall course completion & certificate
	isCompleted, completedCount, totalCount, _ := h.courseService.CheckCourseCompletion(userID, uint(id))
	completedCount, totalWatchSec, avgScore, _ := h.courseService.GetCourseStats(userID, uint(id))

	// Fetch recent on-chain quiz records for stats
	var attempts []models.QuizAttempt
	database.DB.Where("user_id = ?", userID).Order("id desc").Limit(10).Find(&attempts)

	var cert models.Certificate
	hasCertificate := false
	if err := database.DB.Where("user_id = ? AND course_id = ?", userID, id).First(&cert).Error; err == nil {
		hasCertificate = true
	}

	return c.JSON(fiber.Map{
		"success": true,
		"course":  course,
		"lessons": decoratedLessons,
		"summary": fiber.Map{
			"total_lessons":     totalCount,
			"completed_lessons": completedCount,
			"is_completed":      isCompleted,
			"has_certificate":   hasCertificate,
			"certificate":       cert,
			"total_watch_sec":   totalWatchSec,
			"average_score":     avgScore,
			"recent_records":    attempts,
		},
	})
}
