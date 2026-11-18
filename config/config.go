package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port            string
	DBPath          string
	AutoOpenBrowser bool

	// Blockchain settings
	EthRPCURL        string
	ContractAddress  string
	MinterPrivateKey string

	// SMTP Email settings
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// IPFS
	IPFSGateway string

	// Google OAuth / Gmail Login
	GoogleClientID string

	// Auto-Update & Release Tracking
	UpdateRepo        string
	UpdateManifestURL string
}

var AppConfig *Config

func LoadConfig() *Config {
	// Load .env if present (ignore error if not exists)
	_ = godotenv.Load()

	autoOpenStr := getEnv("AUTO_OPEN_BROWSER", "true")
	autoOpen, _ := strconv.ParseBool(autoOpenStr)

	AppConfig = &Config{
		Port:             getEnv("PORT", "80"),
		DBPath:           getEnv("DB_PATH", "elearning.db"),
		AutoOpenBrowser:  autoOpen,
		EthRPCURL:        getEnv("ETH_RPC_URL", ""),
		ContractAddress:  getEnv("CONTRACT_ADDRESS", ""),
		MinterPrivateKey: getEnv("MINTER_PRIVATE_KEY", ""),
		SMTPHost:         getEnv("SMTP_HOST", ""),
		SMTPPort:         getEnv("SMTP_PORT", "587"),
		SMTPUser:         getEnv("SMTP_USER", ""),
		SMTPPass:         getEnv("SMTP_PASS", ""),
		SMTPFrom:         getEnv("SMTP_FROM", "noreply@learnchain.io"),
		IPFSGateway:       getEnv("IPFS_GATEWAY", "https://ipfs.io/ipfs/"),
		GoogleClientID:    getEnv("GOOGLE_CLIENT_ID", ""),
		UpdateRepo:        getEnv("UPDATE_REPO", "Npasak/Learnchain"),
		UpdateManifestURL: getEnv("UPDATE_MANIFEST_URL", ""),
	}

	return AppConfig
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}
