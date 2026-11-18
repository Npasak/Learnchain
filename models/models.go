package models

import (
	"time"

	"gorm.io/gorm"
)

// User represents an account in the platform
type User struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	WalletAddress string         `gorm:"size:64;uniqueIndex;not null" json:"wallet_address"`
	Name          string         `gorm:"size:128" json:"name"`
	Role          string         `gorm:"size:20;default:'learner'" json:"role"` // learner, creator, manager
	Email         string         `gorm:"size:128;index" json:"email"`
	PasswordHash  string         `gorm:"size:255" json:"-"`
	Ward          string         `gorm:"size:64;default:'สว.11'" json:"ward"` // หอสงฆ์, สว.11, สว.12, 8A, 8B, 9A, 9B, 9C
	AvatarURL     string         `gorm:"size:512" json:"avatar_url"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// Course represents an educational course created by a creator
type Course struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Title        string         `gorm:"size:255;not null" json:"title"`
	Summary      string         `gorm:"type:text" json:"summary"`      // เนื้อหาย่อ
	Description  string         `gorm:"type:text" json:"description"`  // รายละเอียดทั้งหมด
	ThumbnailURL string         `gorm:"size:512" json:"thumbnail_url"` // ภาพประกอบคอร์ส
	CreatorID    uint           `gorm:"not null" json:"creator_id"`
	Creator      User           `gorm:"foreignKey:CreatorID" json:"creator,omitempty"`
	Lessons      []Lesson       `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE;" json:"lessons,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Lesson represents an individual lesson inside a Course
type Lesson struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CourseID     uint           `gorm:"not null;index" json:"course_id"`
	Title        string         `gorm:"size:255;not null" json:"title"`
	Summary      string         `gorm:"type:text" json:"summary"`             // เนื้อหาย่อของบทเรียน
	Description  string         `gorm:"type:text" json:"description"`         // รายละเอียดเนื้อหา
	ThumbnailURL string         `gorm:"size:512" json:"thumbnail_url"`        // ภาพประกอบบทเรียน
	YoutubeURL   string         `gorm:"size:512" json:"youtube_url"`          // ลิงก์ YouTube
	DocumentURL  string         `gorm:"size:512" json:"document_url"`                       // ลิงก์เอกสารประกอบ
	DurationSec  float64        `gorm:"default:120" json:"duration_sec"`                    // ระยะเวลาเรียนอย่างน้อยตามที่ผู้สร้างกำหนด (วินาที)
	StudyHours   float64        `gorm:"default:1" json:"study_hours"`                       // จำนวนชั่วโมงการเรียนรู้ที่ระบุบนใบประกาศนียบัตร
	SortOrder    int            `gorm:"not null;default:1" json:"sort_order"`               // ลำดับบทเรียน
	IsHidden     bool           `gorm:"default:false" json:"is_hidden"`       // ซ่อนบทเรียนหรือไม่
	OpenPeriod      string               `gorm:"size:255;default:'เปิดตลอดเวลา'" json:"open_period"` // ระยะเวลาเปิดให้เรียน
	PassScorePct    float64              `gorm:"default:70" json:"pass_score_pct"` // เกณฑ์คะแนนผ่าน (%) ที่ผู้สร้างกำหนด
	DisablePreTest  bool                 `gorm:"default:false" json:"disable_pre_test"`
	DisablePostTest bool                 `gorm:"default:false" json:"disable_post_test"`
	SignatoryIDs    string               `gorm:"type:text" json:"signatory_ids"` // JSON array of Signatory IDs selected by creator
	Quizzes         []Quiz               `gorm:"foreignKey:LessonID;constraint:OnDelete:CASCADE;" json:"quizzes,omitempty"`
	RatingStats  LessonRatingStatsDTO `gorm:"-" json:"rating_stats"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Signatory represents an authorized certificate signer managed by the Manager.
type Signatory struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	FullName     string         `gorm:"size:255;not null" json:"full_name"`
	Position     string         `gorm:"size:255;not null" json:"position"`
	SignatureURL string         `gorm:"type:text" json:"signature_url"`
	CreatedByID  uint           `json:"created_by_id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Quiz represents a pre-test or post-test belonging to a Lesson
type Quiz struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	LessonID  uint           `gorm:"not null;index" json:"lesson_id"`
	Title     string         `gorm:"size:255;not null" json:"title"`
	Type      string         `gorm:"size:30;not null" json:"type"` // "lesson_pre_test" or "lesson_post_test"
	Questions []Question     `gorm:"foreignKey:QuizID;constraint:OnDelete:CASCADE;" json:"questions,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Question represents a question within a Quiz
type Question struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	QuizID       uint           `gorm:"not null;index" json:"quiz_id"`
	QuestionText string         `gorm:"type:text;not null" json:"question_text"`
	Options      []Option       `gorm:"foreignKey:QuestionID;constraint:OnDelete:CASCADE;" json:"options,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Option represents a choice in a Question
type Option struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	QuestionID uint           `gorm:"not null;index" json:"question_id"`
	OptionText string         `gorm:"size:255;not null" json:"option_text"`
	IsCorrect  bool           `gorm:"default:false" json:"is_correct"` // Internal flag; sanitized on learner endpoints!
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// UserLessonProgress tracks a learner's completion status per lesson
type UserLessonProgress struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	UserID         uint           `gorm:"not null;uniqueIndex:idx_user_lesson" json:"user_id"`
	LessonID       uint           `gorm:"not null;uniqueIndex:idx_user_lesson" json:"lesson_id"`
	PreTestPassed  bool           `gorm:"default:false" json:"pre_test_passed"`
	VideoWatched   bool           `gorm:"default:false" json:"video_watched"`
	LastWatchedSec float64        `gorm:"default:0" json:"last_watched_sec"` // เวลาเรียนสะสม
	PostTestPassed bool           `gorm:"default:false" json:"post_test_passed"`
	IsCompleted    bool           `gorm:"default:false" json:"is_completed"`
	OnChainTxHash  string         `gorm:"size:128" json:"on_chain_tx_hash"` // รหัสธุรกรรมบล็อกเชนที่บันทึกการเข้าเรียน
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// QuizAttempt logs every quiz submission attempt by learners
type QuizAttempt struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	QuizID        uint           `gorm:"not null;index" json:"quiz_id"`
	Score         float64        `gorm:"not null" json:"score"`
	TotalScore    float64        `gorm:"not null" json:"total_score"`
	IsPassed      bool           `gorm:"not null" json:"is_passed"`
	OnChainTxHash string         `gorm:"size:128" json:"on_chain_tx_hash"` // รหัสธุรกรรมบันทึกผลการทดสอบลงบล็อกเชน
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// Certificate records minted Soulbound Tokens upon completing a lesson or course with learning statistics
type Certificate struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	UserID            uint           `gorm:"not null;index" json:"user_id"`
	CourseID          uint           `gorm:"not null;index" json:"course_id"`
	LessonID          uint           `gorm:"index" json:"lesson_id"`
	LessonTitle       string         `gorm:"size:255" json:"lesson_title"`
	LessonDurationSec float64        `gorm:"default:0" json:"lesson_duration_sec"`
	StudyHours        float64        `gorm:"default:1" json:"study_hours"`
	TokenID           string         `gorm:"size:64;not null" json:"token_id"`
	TransactionHash   string         `gorm:"size:128;not null" json:"transaction_hash"`
	LessonsCount      int            `gorm:"default:1" json:"lessons_count"`
	TotalWatchSec     float64        `gorm:"default:0" json:"total_watch_sec"`
	AverageScore      float64        `gorm:"default:0" json:"average_score"`
	IssuedAt          time.Time      `json:"issued_at"`
	LessonExists      bool           `gorm:"-" json:"lesson_exists"`
	User              User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Course            Course         `gorm:"foreignKey:CourseID" json:"course,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// LessonAssignment tracks lessons assigned to learners by managers
type LessonAssignment struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	LessonID  uint           `gorm:"not null;index" json:"lesson_id"`
	UserID    uint           `gorm:"not null;index" json:"user_id"`
	Lesson    Lesson         `gorm:"foreignKey:LessonID" json:"lesson,omitempty"`
	User      User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// --- DTOs (Data Transfer Objects) for APIs ---

type OptionSanitized struct {
	ID         uint   `json:"id"`
	QuestionID uint   `json:"question_id"`
	OptionText string `json:"option_text"`
}

type QuestionSanitized struct {
	ID           uint              `json:"id"`
	QuizID       uint              `json:"quiz_id"`
	QuestionText string            `json:"question_text"`
	Options      []OptionSanitized `json:"options"`
}

type QuizSanitized struct {
	ID        uint                `json:"id"`
	LessonID  uint                `json:"lesson_id"`
	Title     string              `json:"title"`
	Type      string              `json:"type"`
	Questions []QuestionSanitized `json:"questions"`
}

type QuizSubmissionRequest struct {
	Answers map[uint]uint `json:"answers"` // QuestionID -> OptionID
}

type VideoProgressRequest struct {
	CurrentSec   float64 `json:"current_sec"`
	PlaybackRate float64 `json:"playback_rate"`
}

type UpdateProfileRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Ward     string `json:"ward"`
	Password string `json:"password"`
}

type UpdateCourseRequest struct {
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Description  string `json:"description"`
	ThumbnailURL string `json:"thumbnail_url"`
	CreatorName  string `json:"creator_name"`
}

type CreateOptionDTO struct {
	OptionText string `json:"option_text"`
	IsCorrect  bool   `json:"is_correct"`
}

type CreateQuestionDTO struct {
	QuestionText string            `json:"question_text"`
	Options      []CreateOptionDTO `json:"options"`
}

type CreateQuizDTO struct {
	Title     string              `json:"title"`
	Type      string              `json:"type"` // "lesson_pre_test" or "lesson_post_test"
	Questions []CreateQuestionDTO `json:"questions"`
}

type CreateLessonRequest struct {
	CourseID     uint            `json:"course_id"`
	CreatorName  string          `json:"creator_name"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	Description  string          `json:"description"`
	ThumbnailURL string          `json:"thumbnail_url"`
	YoutubeURL   string          `json:"youtube_url"`
	DocumentURL  string          `json:"document_url"`
	DurationSec  float64         `json:"duration_sec"`
	StudyHours   float64         `json:"study_hours"`
	SortOrder    int             `json:"sort_order"`
	IsHidden        bool            `json:"is_hidden"`
	OpenPeriod      string          `json:"open_period"`
	PassScorePct    float64         `json:"pass_score_pct"`
	DisablePreTest  bool            `json:"disable_pre_test"`
	DisablePostTest bool            `json:"disable_post_test"`
	SignatoryIDs    string          `json:"signatory_ids"`
	Quizzes         []CreateQuizDTO `json:"quizzes"`
}

type UpdateLessonRequest struct {
	Title           string  `json:"title"`
	Summary         string  `json:"summary"`
	Description     string  `json:"description"`
	ThumbnailURL    string  `json:"thumbnail_url"`
	YoutubeURL      string  `json:"youtube_url"`
	DocumentURL     string  `json:"document_url"`
	DurationSec     float64 `json:"duration_sec"`
	StudyHours      float64 `json:"study_hours"`
	SortOrder       int     `json:"sort_order"`
	IsHidden        *bool   `json:"is_hidden"`
	OpenPeriod      string  `json:"open_period"`
	PassScorePct    float64 `json:"pass_score_pct"`
	DisablePreTest  *bool   `json:"disable_pre_test"`
	DisablePostTest *bool   `json:"disable_post_test"`
	SignatoryIDs    *string `json:"signatory_ids"`
}

type AssignLearnerRequest struct {
	LessonID uint   `json:"lesson_id"`
	UserID   uint   `json:"user_id"`
	Ward     string `json:"ward"`
}

type GoogleLoginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	Name       string `json:"name"`
	Ward       string `json:"ward"`
	Role       string `json:"role"`
	AvatarURL  string `json:"avatar_url"`
	Credential string `json:"credential"`
}

type RegisterUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Ward     string `json:"ward"`
	Role     string `json:"role"`
}

type EmailLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// LessonRating stores a learner's 1-5 star rating & optional comment for a lesson (1 per user per lesson, anonymous in reports)
type LessonRating struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	LessonID     uint           `gorm:"not null;uniqueIndex:idx_user_lesson_rating" json:"lesson_id"`
	UserID       uint           `gorm:"not null;uniqueIndex:idx_user_lesson_rating" json:"-"`
	Rating       int            `gorm:"not null" json:"rating"` // 1..5
	Comment      string         `gorm:"type:text" json:"comment"`
	IsReported         bool           `gorm:"default:false" json:"is_reported"`
	ReportReason       string         `gorm:"type:text" json:"report_reason,omitempty"`
	ReportedBy         string         `json:"reported_by,omitempty"`
	ReportedAt         *time.Time     `json:"reported_at,omitempty"`
	CommentModeratedAt *time.Time     `json:"comment_moderated_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// LessonRatingCommentDTO holds an anonymous learner comment + star rating.
type LessonRatingCommentDTO struct {
	ID           uint      `json:"id"`
	LessonID     uint      `json:"lesson_id"`
	Rating       int       `json:"rating"`
	Comment      string    `json:"comment"`
	IsReported   bool      `json:"is_reported"`
	ReportReason string    `json:"report_reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// LessonRatingStatsDTO holds aggregated anonymous rating stats (1-5 stars breakdown, average & comments)
type LessonRatingStatsDTO struct {
	LessonID      uint                     `json:"lesson_id"`
	AverageRating float64                  `json:"average_rating"`
	TotalRatings  int64                    `json:"total_ratings"`
	StarCounts    map[int]int              `json:"star_counts"` // 1..5 -> count
	MyRating      int                      `json:"my_rating,omitempty"`
	MyComment     string                   `json:"my_comment,omitempty"`
	Comments      []LessonRatingCommentDTO `json:"comments,omitempty"`
}
