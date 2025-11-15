package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/examples"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ProcessChannels
//   - session is stored in a local file (./session/phone<number>.json).
func ProcessChannels(
	ctx context.Context,
	apiID int,
	apiHash string,
	phone string,
	srcChatID int64,
	targetChatID int64,
	logger *zap.SugaredLogger,
) error {
	sessionDir := filepath.Join("session", sanitizePhone(phone))
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return errors.Wrap(err, "create session dir")
	}
	sessionFile := filepath.Join(sessionDir, "session.json")

	waiter := floodwait.NewWaiter().WithCallback(func(ctx context.Context, f floodwait.FloodWait) {
		logger.Warnw("Flood wait triggered", "duration", f.Duration)
	})

	tgClient := telegram.NewClient(apiID, apiHash, telegram.Options{
		Logger:         zap.NewNop(),
		SessionStorage: &session.FileStorage{Path: sessionFile},
		Middlewares:    []telegram.Middleware{waiter},
	})

	myClient := &Client{
		client: tgClient,
		logger: logger,
	}
	flow := auth.NewFlow(examples.Terminal{PhoneNumber: phone}, auth.SendCodeOptions{})

	limiter := rate.NewLimiter(rate.Every(time.Minute/20), 1)

	return waiter.Run(ctx, func(ctx context.Context) error {
		return tgClient.Run(ctx, func(ctx context.Context) error {
			if err := tgClient.Auth().IfNecessary(ctx, flow); err != nil {
				return errors.Wrap(err, "auth")
			}
			myClient.api = tgClient.API()

			messages, err := myClient.fetchChannelMessages(ctx, srcChatID)
			if err != nil {
				return errors.Wrap(err, "fetch messages")
			}

			targetChannel, err := myClient.getPeerChannel(ctx, targetChatID)
			if err != nil {
				return errors.Wrap(err, "target channel")
			}

			var (
				currentGroupID int64
				albumBuf       []*tg.Message
			)

			// flushAlbum send current album
			flushAlbum := func() error {
				if len(albumBuf) == 0 {
					return nil
				}
				var items []MultiMediaItem
				for _, m := range albumBuf {
					if m.Media == nil {
						fmt.Println("СООБЩЕНИЕ БЕЗ МЕДИА")
						if err := limiter.Wait(ctx); err != nil {
							return errors.Wrap(err, "rate limit wait")
						}
						if _, err := myClient.sendTextMessage(ctx, targetChannel, m.Message, m.Entities); err != nil {
							return err
						}
						continue
					}
					switch media := m.Media.(type) {
					case *tg.MessageMediaPhoto:
						photo := media.Photo
						if photo != nil {
							if p, ok := photo.AsNotEmpty(); ok {
								items = append(items, MultiMediaItem{
									Media: &tg.InputMediaPhoto{
										ID: p.AsInput(),
									},
									Caption:  m.Message,
									Entities: m.Entities,
								})
							}
						}
					case *tg.MessageMediaDocument:
						doc := media.Document
						if doc != nil {
							if d, ok := doc.AsNotEmpty(); ok {
								items = append(items, MultiMediaItem{
									Media: &tg.InputMediaDocument{
										ID: d.AsInput(),
									},
									Caption:  m.Message,
									Entities: m.Entities,
								})
							}
						}
					default:
						fmt.Println("Unhandled media type:", reflect.TypeOf(media))
					}
				}
				albumBuf = nil
				currentGroupID = 0

				if len(items) == 0 {
					return nil
				}
				if err := limiter.Wait(ctx); err != nil {
					return errors.Wrap(err, "rate limit wait")
				}
				if _, err := myClient.sendMultiMedia(ctx, targetChannel, items); err != nil {
					if tg.IsChatForwardsRestricted(err) {
						// TODO: Тут ошибка выскакивает на некоторых сообщениях надо исправлять логику
						fmt.Println(tg.IsChatForwardsRestricted(err))
						fmt.Println("ChatForwardsRestricted error:", err)
					}
				}
				return nil
			}

			for _, msg := range messages {
				if msg.GroupedID == 0 {
					if err := flushAlbum(); err != nil {
						return err
					}
					// Single media or text
					if msg.Media != nil {
						switch media := msg.Media.(type) {
						case *tg.MessageMediaPhoto:
							if photo, ok := media.Photo.AsNotEmpty(); ok {
								if err := limiter.Wait(ctx); err != nil {
									return errors.Wrap(err, "rate limit wait")
								}
								if _, err := myClient.sendMedia(ctx, targetChannel, &tg.InputMediaPhoto{
									ID: photo.AsInput(),
								}, msg.Message, msg.Entities); err != nil {
									return err
								}
							}
						case *tg.MessageMediaDocument:
							if doc, ok := media.Document.AsNotEmpty(); ok {
								if err := limiter.Wait(ctx); err != nil {
									return errors.Wrap(err, "rate limit wait")
								}
								if _, err := myClient.sendMedia(ctx, targetChannel, &tg.InputMediaDocument{
									ID: doc.AsInput(),
								}, msg.Message, msg.Entities); err != nil {
									return err
								}
							}
						default:
							fmt.Println("Unhandled media type:", reflect.TypeOf(media))
						}
					} else {
						if err := limiter.Wait(ctx); err != nil {
							return errors.Wrap(err, "rate limit wait")
						}
						if _, err := myClient.sendTextMessage(ctx, targetChannel, msg.Message, msg.Entities); err != nil {
							return err
						}
					}
					continue
				}

				// If GroupedID != 0, message is part of album
				if currentGroupID == 0 {
					currentGroupID = msg.GroupedID
					albumBuf = []*tg.Message{msg}
					continue
				}
				if msg.GroupedID == currentGroupID {
					albumBuf = append(albumBuf, msg)
				} else {
					if err := flushAlbum(); err != nil {
						return err
					}
					currentGroupID = msg.GroupedID
					albumBuf = []*tg.Message{msg}
				}
			}

			if err := flushAlbum(); err != nil {
				return err
			}

			return nil
		})
	})
}
