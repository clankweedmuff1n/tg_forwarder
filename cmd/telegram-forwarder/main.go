package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/viper"

	"tg_forwarder/internal/telegram"
	"tg_forwarder/pkg/config"
	"tg_forwarder/pkg/logging"
)

func main() {
	config.InitConfig()
	rand.Seed(time.Now().UnixNano())

	logger := logging.NewLogger()
	defer func() { _ = logger.Sync() }()

	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	apiID := viper.GetInt("API_ID")
	apiHash := viper.GetString("API_HASH")
	phone := viper.GetString("PHONE")
	if apiID == 0 || apiHash == "" || phone == "" {
		logger.Fatal("Missing required config: API_ID, API_HASH, PHONE must be set.")
	}

	srcChatID := viper.GetInt64("SRC_CHAT")
	dstChatID := viper.GetInt64("DST_CHAT")
	if srcChatID == 0 || dstChatID == 0 {
		logger.Warn("SRC_CHAT or DST_CHAT not set. The program may not function as expected.")
	}

	if err := telegram.ProcessChannels(ctx, apiID, apiHash, phone, srcChatID, dstChatID, logger); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}

	logger.Infow("All messages forwarded successfully.")
	fmt.Println("Done.")
}
