package config

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"
)

func InitConfig() {
	viper.SetDefault("API_ID", 0)
	viper.SetDefault("API_HASH", "")
	viper.SetDefault("PHONE", "")
	viper.SetDefault("SRC_CHAT", 0)
	viper.SetDefault("DST_CHAT", 0)

	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	_ = viper.ReadInConfig()

	viper.AutomaticEnv()

	flag.Int("api-id", 0, "Telegram API ID (override .env)")
	flag.String("api-hash", "", "Telegram API Hash (override .env)")
	flag.String("phone", "", "Telegram phone number (override .env)")
	flag.Int64("src-chat", 0, "Source chat ID (override .env)")
	flag.Int64("dst-chat", 0, "Destination chat ID (override .env)")
}

func ValidateConfig() error {
	if viper.GetInt("API_ID") == 0 || viper.GetString("API_HASH") == "" {
		return fmt.Errorf("missing API_ID or API_HASH")
	}
	return nil
}
