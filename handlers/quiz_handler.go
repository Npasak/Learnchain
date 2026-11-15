package handlers

import (
	"fmt"
	"strconv"

	"elearning-sbt/database"
	"elearning-sbt/middleware"
	"elearning-sbt/models"
	"elearning-sbt/services"

	"github.com/gofiber/fiber/v2"
)

type QuizHandler struct {
	courseService     *services.CourseService
	blockchainService *services.BlockchainService
}

func NewQuizHandler(cs *services.CourseService, bs *services.BlockchainService) *QuizHandler {
	return &QuizHandler{courseService: cs, blockchainService: bs}
}

// GetQuiz retrieves quiz questions with IsCorrect strictly hidden from the frontend
func (h *QuizHandler) GetQuiz(c *fiber.Ctx) error {
	quizIDStr := c.Params("quizId")
	quizID, err := strconv.ParseUint(quizIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid quiz ID",
		})
	}

	var quiz models.Quiz
	err = database.DB.Preload("Questions").
		Preload("Questions.Options").
		First(&quiz, quizID).Error
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Quiz not found",
		})
	}

	// Sanitize output: Strip out IsCorrect field from each Option
	sanitizedQuestions := make([]models.QuestionSanitized, 0, len(quiz.Questions))
	for _, q := range quiz.Questions {
		sanitizedOptions := make([]models.OptionSanitized, 0, len(q.Options))
		for _, opt := range q.Options {
			sanitizedOptions = append(sanitizedOptions, models.OptionSanitized{
				ID:         opt.ID,
				QuestionID: opt.QuestionID,
				OptionText: opt.OptionText,
			})
		}
		sanitizedQuestions = append(sanitizedQuestions, models.QuestionSanitized{
			ID:           q.ID,
			QuizID:       q.QuizID,
			QuestionText: q.QuestionText,
			Options:      sanitizedOptions,
		})
	}

	sanitizedQuiz := models.QuizSanitized{
		ID:        quiz.ID,
		LessonID:  quiz.LessonID,
		Title:     quiz.Title,
		Type:      quiz.Type,
		Questions: sanitizedQuestions,
	}

	return c.JSON(fiber.Map{
		"success": true,
		"quiz":    sanitizedQuiz,
	})
}

// SubmitQuiz grades the student's submission against the database, records QuizAttempt, and updates progress
func (h *QuizHandler) SubmitQuiz(c *fiber.Ctx) error {
	quizIDStr := c.Params("quizId")
	quizID, err := strconv.ParseUint(quizIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid quiz ID",
		})
	}

	var req models.QuizSubmissionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid answers payload",
		})
	}

	// Load full quiz with correct answer options from database
	var quiz models.Quiz
	err = database.DB.Preload("Questions").
		Preload("Questions.Options").
		First(&quiz, quizID).Error
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Quiz not found",
		})
	}

	totalQuestions := float64(len(quiz.Questions))
	if totalQuestions == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Quiz has no questions",
		})
	}

	// Evaluate student answers
	correctCount := 0.0
	for _, q := range quiz.Questions {
		userSelectedOptionID := req.Answers[q.ID]
		for _, opt := range q.Options {
			if opt.ID == userSelectedOptionID && opt.IsCorrect {
				correctCount++
				break
			}
		}
	}

	var targetLesson models.Lesson
	database.DB.First(&targetLesson, quiz.LessonID)
	passThreshold := targetLesson.PassScorePct
	if passThreshold <= 0 {
		passThreshold = 70.0
	}

	scorePercent := (correctCount / totalQuestions) * 100.0
	isPassed := scorePercent >= passThreshold

	userID := middleware.ResolveUserID(c)

	var user models.User
	database.DB.First(&user, userID)

	// ถ้าเป็นผู้สร้าง (Creator): โหมดทดสอบระบบ ไม่บันทึกลงบล็อกเชนและไม่เก็บสถิติ QuizAttempt
	if user.Role == "creator" {
		return c.JSON(fiber.Map{
			"success":          true,
			"score":            correctCount,
			"total_score":      totalQuestions,
			"percentage":       scorePercent,
			"pass_score_pct":   passThreshold,
			"is_passed":        isPassed,
			"is_creator_test":  true,
			"on_chain_tx_hash": "",
			"message": func() string {
				if isPassed {
					return fmt.Sprintf("🎉 [โหมดทดสอบผู้สร้าง] คำตอบถูกต้องผ่านเกณฑ์ %.0f%% (ไม่เก็บสถิติและไม่ส่งบล็อกเชน)", passThreshold)
				}
				return fmt.Sprintf("⚠️ [โหมดทดสอบผู้สร้าง] คะแนนยังไม่ถึงเกณฑ์ %.0f%% (ไม่เก็บสถิติและไม่ส่งบล็อกเชน)", passThreshold)
			}(),
		})
	}

	txHash, _ := h.blockchainService.RecordQuizOnChain(user.WalletAddress, uint(quizID), correctCount, totalQuestions, isPassed)

	// Record QuizAttempt
	attempt := models.QuizAttempt{
		UserID:        userID,
		QuizID:        uint(quizID),
		Score:         correctCount,
		TotalScore:    totalQuestions,
		IsPassed:      isPassed,
		OnChainTxHash: txHash,
	}
	database.DB.Create(&attempt)

	// Update UserLessonProgress
	progress, err := h.courseService.GetOrCreateProgress(userID, quiz.LessonID)
	if err == nil {
		if isPassed {
			if quiz.Type == "lesson_pre_test" {
				progress.PreTestPassed = true
			} else if quiz.Type == "lesson_post_test" {
				progress.PostTestPassed = true
			}
		}

		var lesson models.Lesson
		if database.DB.First(&lesson, quiz.LessonID).Error == nil {
			var preCount, postCount int64
			database.DB.Model(&models.Quiz{}).Where("lesson_id = ? AND type = ?", lesson.ID, "lesson_pre_test").Count(&preCount)
			database.DB.Model(&models.Quiz{}).Where("lesson_id = ? AND type = ?", lesson.ID, "lesson_post_test").Count(&postCount)

			preOK := lesson.DisablePreTest || preCount == 0 || progress.PreTestPassed
			postOK := lesson.DisablePostTest || postCount == 0 || progress.PostTestPassed
			videoOK := lesson.DurationSec == 0 || progress.VideoWatched

			if preOK && postOK && videoOK {
				progress.IsCompleted = true
			}
		}
		database.DB.Save(progress)

		if progress.IsCompleted {
			_, _ = h.courseService.EnsureLessonCertificate(userID, quiz.LessonID, h.blockchainService)
		}
	}

	// Check if this action completed the entire course
	var lesson models.Lesson
	database.DB.First(&lesson, quiz.LessonID)
	courseCompleted, completedLessons, totalLessons, _ := h.courseService.CheckCourseCompletion(userID, lesson.CourseID)

	return c.JSON(fiber.Map{
		"success":           true,
		"score":             correctCount,
		"total_score":       totalQuestions,
		"percentage":        scorePercent,
		"pass_score_pct":    passThreshold,
		"is_passed":         isPassed,
		"on_chain_tx_hash":  txHash,
		"lesson_progress":   progress,
		"course_completed":  courseCompleted,
		"completed_lessons": completedLessons,
		"total_lessons":     totalLessons,
		"message": func() string {
			if isPassed {
				return "ยินดีด้วยครับ! คุณสอบผ่านการประเมินความรู้ และระบบได้บันทึกคะแนนลงบล็อกเชนแล้ว"
			}
			return fmt.Sprintf("คะแนนยังไม่ถึงเกณฑ์ %.0f%% ผลการประเมินถูกบันทึกลงบล็อกเชนแล้ว กรุณาทบทวนบทเรียนและสอบใหม่อีกครั้งครับ", passThreshold)
		}(),
	})
}
