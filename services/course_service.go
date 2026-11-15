package services

import (
	"errors"
	"fmt"
	"math"
	"time"

	"elearning-sbt/models"

	"gorm.io/gorm"
)

type CourseService struct {
	db *gorm.DB
}

func NewCourseService(db *gorm.DB) *CourseService {
	return &CourseService{db: db}
}

// CheckLessonAccess verifies access: ทุกบทเรียนเป็นอิสระต่อกัน สามารถเลือกเรียนบทใดก็ได้ตามต้องการ
func (s *CourseService) CheckLessonAccess(userID, lessonID uint) (bool, string, error) {
	var currentLesson models.Lesson
	if err := s.db.First(&currentLesson, lessonID).Error; err != nil {
		return false, "Lesson not found", err
	}

	// บทเรียนเป็นอิสระต่อกัน (ไม่บังคับเรียนตามลำดับ)
	return true, "", nil
}

// GetOrCreateProgress retrieves or initializes a learner's progress for a lesson
func (s *CourseService) GetOrCreateProgress(userID, lessonID uint) (*models.UserLessonProgress, error) {
	var progress models.UserLessonProgress
	// ใช้ Unscoped เพื่อตรวจหาแถวข้อมูลทั้งหมด ป้องกันข้อผิดพลาด SQLite Unique Constraint จากแถวที่ Soft-Deleted
	err := s.db.Unscoped().Where("user_id = ? AND lesson_id = ?", userID, lessonID).First(&progress).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			progress = models.UserLessonProgress{
				UserID:         userID,
				LessonID:       lessonID,
				PreTestPassed:  false,
				VideoWatched:   false,
				LastWatchedSec: 0,
				PostTestPassed: false,
				IsCompleted:    false,
			}
			if err := s.db.Create(&progress).Error; err != nil {
				return nil, err
			}
			return &progress, nil
		}
		return nil, err
	}

	// ถ้าแถวเคยถูก Soft-Delete ให้กู้คืน (Un-delete) และรีเซ็ตค่าเป็น 0
	if progress.DeletedAt.Valid {
		progress.DeletedAt = gorm.DeletedAt{}
		progress.PreTestPassed = false
		progress.VideoWatched = false
		progress.LastWatchedSec = 0
		progress.PostTestPassed = false
		progress.IsCompleted = false
		progress.OnChainTxHash = ""
		if err := s.db.Unscoped().Save(&progress).Error; err != nil {
			return nil, err
		}
	}

	return &progress, nil
}

// UpdateVideoProgress tracks playback time and enforces anti-skip verification
func (s *CourseService) UpdateVideoProgress(userID, lessonID uint, currentSec, playbackRate float64) (*models.UserLessonProgress, error) {
	var lesson models.Lesson
	if err := s.db.First(&lesson, lessonID).Error; err != nil {
		return nil, errors.New("lesson not found")
	}

	progress, err := s.GetOrCreateProgress(userID, lessonID)
	if err != nil {
		return nil, err
	}

	// Anti-Skip Verification Logic:
	// A user cannot jump forward more than 15 seconds ahead of their last verified position in a single update
	maxAllowedJump := 15.0
	if playbackRate > 1.0 {
		maxAllowedJump *= playbackRate
	}

	if currentSec > progress.LastWatchedSec+maxAllowedJump && !progress.VideoWatched {
		// Detected unnatural skip ahead - reject jumping forward
		return progress, fmt.Errorf("anti-skip guard: cannot fast-forward past unviewed content (Last: %.1fs, Attempted: %.1fs)", progress.LastWatchedSec, currentSec)
	}

	// Advance last watched second if user watched further
	if currentSec > progress.LastWatchedSec {
		progress.LastWatchedSec = math.Min(currentSec, lesson.DurationSec)
	}

	// Check if video is considered completed (watched >= 90% or reached end)
	threshold := lesson.DurationSec * 0.90
	if lesson.DurationSec <= 0 || progress.LastWatchedSec >= threshold {
		progress.VideoWatched = true
	}

	// Evaluate overall lesson completion:
	// Requires PreTestPassed, VideoWatched, and PostTestPassed (if respective quizzes exist)
	s.evaluateLessonCompletion(&lesson, progress)

	if err := s.db.Save(progress).Error; err != nil {
		return nil, err
	}

	return progress, nil
}

// evaluateLessonCompletion checks if all requirements for the lesson are satisfied
func (s *CourseService) evaluateLessonCompletion(lesson *models.Lesson, progress *models.UserLessonProgress) {
	var preQuizCount int64
	var postQuizCount int64

	s.db.Model(&models.Quiz{}).Where("lesson_id = ? AND type = ?", lesson.ID, "lesson_pre_test").Count(&preQuizCount)
	s.db.Model(&models.Quiz{}).Where("lesson_id = ? AND type = ?", lesson.ID, "lesson_post_test").Count(&postQuizCount)

	preOK := lesson.DisablePreTest || preQuizCount == 0 || progress.PreTestPassed
	postOK := lesson.DisablePostTest || postQuizCount == 0 || progress.PostTestPassed
	videoOK := lesson.DurationSec == 0 || progress.VideoWatched

	if preOK && postOK && videoOK {
		progress.IsCompleted = true
	}
}

// CheckCourseCompletion checks if all lessons in the course have been completed by the user
func (s *CourseService) CheckCourseCompletion(userID, courseID uint) (bool, int, int, error) {
	var lessons []models.Lesson
	if err := s.db.Where("course_id = ?", courseID).Find(&lessons).Error; err != nil {
		return false, 0, 0, err
	}

	if len(lessons) == 0 {
		return false, 0, 0, nil
	}

	completedCount := 0
	for _, l := range lessons {
		var p models.UserLessonProgress
		err := s.db.Where("user_id = ? AND lesson_id = ?", userID, l.ID).First(&p).Error
		if err == nil && p.IsCompleted {
			completedCount++
		}
	}

	isCompleted := completedCount == len(lessons)
	return isCompleted, completedCount, len(lessons), nil
}

// GetCourseStats calculates completed lessons, total watch seconds, and average quiz percentage across all lessons
func (s *CourseService) GetCourseStats(userID, courseID uint) (int, float64, float64, error) {
	var lessons []models.Lesson
	if err := s.db.Find(&lessons).Error; err != nil {
		return 0, 0, 0, err
	}

	completedLessons := 0
	totalWatchSec := 0.0
	for _, l := range lessons {
		var p models.UserLessonProgress
		if err := s.db.Where("user_id = ? AND lesson_id = ?", userID, l.ID).First(&p).Error; err == nil {
			if p.IsCompleted {
				completedLessons++
			}
			totalWatchSec += p.LastWatchedSec
		}
	}

	// Calculate average score for quizzes attempted by this user
	var attempts []models.QuizAttempt
	s.db.Where("user_id = ?", userID).Find(&attempts)

	avgScore := 0.0
	if len(attempts) > 0 {
		sumPct := 0.0
		for _, a := range attempts {
			if a.TotalScore > 0 {
				sumPct += (a.Score / a.TotalScore) * 100.0
			}
		}
		avgScore = sumPct / float64(len(attempts))
	}

	return completedLessons, totalWatchSec, avgScore, nil
}

// EnsureLessonCertificate mints and saves a Soulbound Certificate for a specific completed lesson if not already issued
func (s *CourseService) EnsureLessonCertificate(userID, lessonID uint, bs *BlockchainService) (*models.Certificate, error) {
	var existing models.Certificate
	if err := s.db.Preload("User").Preload("Course").Where("user_id = ? AND lesson_id = ?", userID, lessonID).First(&existing).Error; err == nil {
		var lesson models.Lesson
		if s.db.First(&lesson, lessonID).Error == nil {
			updated := false
			if existing.LessonTitle == "" {
				existing.LessonTitle = lesson.Title
				updated = true
			}
			if existing.LessonDurationSec != lesson.DurationSec && lesson.DurationSec > 0 {
				existing.LessonDurationSec = lesson.DurationSec
				updated = true
			}
			hours := lesson.StudyHours
			if hours <= 0 {
				hours = 1
			}
			if existing.StudyHours != hours {
				existing.StudyHours = hours
				updated = true
			}
			if updated {
				s.db.Save(&existing)
			}
		}
		return &existing, nil
	}

	var lesson models.Lesson
	if err := s.db.First(&lesson, lessonID).Error; err != nil {
		return nil, err
	}

	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, err
	}

	var progress models.UserLessonProgress
	s.db.Where("user_id = ? AND lesson_id = ?", userID, lessonID).First(&progress)

	// Calculate average quiz score for this lesson
	var attempts []models.QuizAttempt
	s.db.Table("quiz_attempts").
		Joins("JOIN quizzes ON quizzes.id = quiz_attempts.quiz_id").
		Where("quiz_attempts.user_id = ? AND quizzes.lesson_id = ? AND quiz_attempts.is_passed = ?", userID, lessonID, true).
		Find(&attempts)

	avgScore := 100.0
	if len(attempts) > 0 {
		sumPct := 0.0
		for _, a := range attempts {
			if a.TotalScore > 0 {
				sumPct += (a.Score / a.TotalScore) * 100.0
			}
		}
		avgScore = sumPct / float64(len(attempts))
	}

	metadataURI := fmt.Sprintf("ipfs://bafkreicertificate-lesson-%d-user-%d-metadata.json", lessonID, userID)
	txHash := progress.OnChainTxHash
	tokenID := fmt.Sprintf("SBT-%d-%d-%d", lesson.CourseID, lessonID, userID)

	if bs != nil {
		if mintRes, err := bs.MintSoulboundCertificate(user.WalletAddress, lesson.CourseID, 1, progress.LastWatchedSec, avgScore, metadataURI); err == nil && mintRes != nil {
			tokenID = mintRes.TokenID
			txHash = mintRes.TransactionHash
		}
	}
	if txHash == "" {
		txHash = fmt.Sprintf("0x%064x", time.Now().UnixNano())
	}

	studyHours := lesson.StudyHours
	if studyHours <= 0 {
		studyHours = 1
	}

	cert := models.Certificate{
		UserID:            user.ID,
		CourseID:          lesson.CourseID,
		LessonID:          lesson.ID,
		LessonTitle:       lesson.Title,
		LessonDurationSec: lesson.DurationSec,
		StudyHours:        studyHours,
		TokenID:           tokenID,
		TransactionHash:   txHash,
		LessonsCount:      1,
		TotalWatchSec:     progress.LastWatchedSec,
		AverageScore:      avgScore,
		IssuedAt:          time.Now(),
	}

	if err := s.db.Create(&cert).Error; err != nil {
		return nil, err
	}

	s.db.Preload("User").Preload("Course").First(&cert, cert.ID)
	return &cert, nil
}

// EnsureAllUserCertificates ensures every completed lesson of the user has a corresponding Certificate record
func (s *CourseService) EnsureAllUserCertificates(userID uint, bs *BlockchainService) ([]models.Certificate, error) {
	var completedProgresses []models.UserLessonProgress
	s.db.Where("user_id = ? AND is_completed = ?", userID, true).Find(&completedProgresses)

	for _, p := range completedProgresses {
		_, _ = s.EnsureLessonCertificate(userID, p.LessonID, bs)
	}

	var certs []models.Certificate
	s.db.Preload("Course").Preload("User").Where("user_id = ?", userID).Order("id desc").Find(&certs)
	return certs, nil
}
