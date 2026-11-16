package database

import (
	"log"

	"elearning-sbt/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB initializes SQLite database connection and auto-migrates models
func InitDB(dbPath string) *gorm.DB {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	log.Println("📦 Connected to SQLite database successfully.")

	// Drop legacy unique index idx_user_course so multiple lesson certificates can be issued per user
	DB.Exec("DROP INDEX IF EXISTS idx_user_course")

	// AutoMigrate all models
	err = DB.AutoMigrate(
		&models.User{},
		&models.Course{},
		&models.Lesson{},
		&models.Quiz{},
		&models.Question{},
		&models.Option{},
		&models.UserLessonProgress{},
		&models.QuizAttempt{},
		&models.Certificate{},
		&models.LessonAssignment{},
		&models.LessonRating{},
		&models.Signatory{},
	)
	if err != nil {
		log.Fatalf("❌ AutoMigration failed: %v", err)
	}

	log.Println("✅ Database AutoMigrate completed.")

	// Seed demo data if database is empty
	seedDemoData(DB)
	cleanupRetiredDemoLessons(DB)

	// Ensure at least one Manager exists in the system (if 0 managers exist, promote the first Creator with an email)
	var mgrCount int64
	DB.Model(&models.User{}).Where("role = ?", "manager").Count(&mgrCount)
	if mgrCount == 0 {
		var firstCreator models.User
		if err := DB.Where("role = ? AND email != ''", "creator").Order("id desc").First(&firstCreator).Error; err == nil {
			mgrUser := models.User{
				WalletAddress: firstCreator.WalletAddress + "-mgr",
				Name:          firstCreator.Name,
				Role:          "manager",
				Email:         firstCreator.Email,
				AvatarURL:     firstCreator.AvatarURL,
			}
			DB.Create(&mgrUser)
			log.Printf("👔 Bootstrapped initial Manager account from Creator: %s (%s)", firstCreator.Name, firstCreator.Email)
		}
	}

	return DB
}

// RetiredDemoLessonTitles lists legacy Web3/SBT sample lessons that should never be re-seeded.
var RetiredDemoLessonTitles = []string{
	"บทที่ 1: พื้นฐานเทคโนโลยี Web3 และกระเป๋าดิจิทัล (Wallets)",
	"บทที่ 2: การพัฒนา Soulbound Token (SBT) บน Solidity และห้ามโอนย้าย",
}

func cleanupRetiredDemoLessons(db *gorm.DB) {
	res := db.Where("title IN ?", RetiredDemoLessonTitles).Delete(&models.Lesson{})
	if res.Error == nil && res.RowsAffected > 0 {
		log.Printf("🧹 Removed %d retired Web3/SBT demo lesson(s) from database", res.RowsAffected)
	}
}

// seedDemoData populates default accounts and course for instant out-of-the-box testing.
func seedDemoData(db *gorm.DB) {
	var userCount int64
	db.Model(&models.User{}).Count(&userCount)
	if userCount > 0 {
		return
	}

	log.Println("🌱 Seeding initial demo accounts and course...")

	// 1. Create Creator & Demo Learner
	creator := models.User{
		WalletAddress: "0x71C83a451D14feB383021f1d1326410408544955",
		Name:          "ผศ.ดร. ธนวัฒน์ บล็อกเชนวิทยา (Senior Blockchain Architect)",
		Role:          "creator",
		Email:         "creator@learnchain.io",
	}
	db.Create(&creator)

	learner := models.User{
		WalletAddress: "0x90F79bf6EB2c4f870365E785982E1f101E93b906",
		Name:          "สมศักดิ์ ผู้เรียนรู้ Web3",
		Role:          "learner",
		Email:         "learner@example.com",
	}
	db.Create(&learner)

	// 2. Create Default Course
	course := models.Course{
		Title:        "หลักสูตรการพยาบาล",
		Summary:      "บทเรียนออนไลน์ทางการพยาบาลและวุฒิบัตรอิเล็กทรอนิกส์",
		Description:  "ระบบการเรียนรู้ทางการพยาบาลพร้อมวุฒิบัตรอิเล็กทรอนิกส์ (SBT)",
		ThumbnailURL: "https://images.unsplash.com/photo-1639762681485-074b7f938ba0?q=80&w=800&auto=format&fit=crop",
		CreatorID:    creator.ID,
	}
	db.Create(&course)

	log.Println("✨ Initial accounts and course seeded successfully!")
}
