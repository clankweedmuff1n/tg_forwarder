package telegram

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"
)

type Client struct {
	client *telegram.Client
	api    *tg.Client
	logger *zap.SugaredLogger

	mu sync.Mutex
}

type MultiMediaItem struct {
	Media    tg.InputMediaClass
	Caption  string
	Entities []tg.MessageEntityClass
}

func (client *Client) fetchChannelMessages(ctx context.Context, chatID int64) ([]*tg.Message, error) {
	var result []*tg.Message

	inputPeer, err := client.getPeerChannel(ctx, chatID)
	if err != nil {
		return nil, errors.Wrap(err, "getPeerChannel")
	}
	req := &tg.MessagesGetHistoryRequest{Peer: inputPeer}
	const delayTime = 100 * time.Millisecond

	for {
		historyClass, err := client.api.MessagesGetHistory(ctx, req)
		if err != nil {
			if flood, _ := tgerr.FloodWait(ctx, err); flood {
				continue
			}
			return nil, err
		}
		history, ok := historyClass.(*tg.MessagesChannelMessages)
		if !ok || len(history.Messages) == 0 {
			break
		}
		for _, msg := range history.Messages {
			if m, ok := msg.(*tg.Message); ok {
				result = append(result, m)
			}
		}
		req.OffsetID = history.Messages[len(history.Messages)-1].GetID()

		time.Sleep(delayTime)
	}

	length := len(result)
	for i := 0; i < length/2; i++ {
		result[i], result[length-1-i] = result[length-1-i], result[i]
	}
	return result, nil
}

func (client *Client) getPeerChannel(ctx context.Context, channelID int64) (*tg.InputPeerChannel, error) {
	inputChannel := &tg.InputChannel{
		ChannelID:  channelID,
		AccessHash: 0,
	}
	channels, err := client.api.ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch channel: %w", err)
	}
	ch := channels.GetChats()[0].(*tg.Channel)
	return &tg.InputPeerChannel{
		ChannelID:  channelID,
		AccessHash: ch.AccessHash,
	}, nil
}

func (client *Client) sendTextMessage(ctx context.Context, peer tg.InputPeerClass, text string, entities []tg.MessageEntityClass) (int, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	maxInt := new(big.Int).Lsh(big.NewInt(1), 63)
	randomId, err := rand.Int(rand.Reader, maxInt)
	if err != nil {
		return 0, errors.Wrap(err, "RandomId")
	}

	req := &tg.MessagesSendMessageRequest{
		Peer:      peer,
		Message:   text,
		Entities:  entities,
		NoWebpage: true,
		RandomID:  randomId.Int64(),
	}

	resp, err := client.api.MessagesSendMessage(ctx, req)
	if err != nil {
		fmt.Println("Error", err)
	}

	upd, ok := resp.(*tg.Updates)
	if !ok {
		return 0, fmt.Errorf("unexpected response type from sendMessage")
	}
	for _, update := range upd.Updates {
		switch update := update.(type) {
		case *tg.UpdateMessageID:
			return update.ID, nil
		}
	}
	return 0, fmt.Errorf("could not find new message ID in updates (message)")
}

func (client *Client) sendMedia(ctx context.Context, peer tg.InputPeerClass, media tg.InputMediaClass, caption string, entities []tg.MessageEntityClass) (int, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	maxInt := new(big.Int).Lsh(big.NewInt(1), 63)
	randomId, err := rand.Int(rand.Reader, maxInt)
	if err != nil {
		return 0, errors.Wrap(err, "RandomId")
	}

	req := &tg.MessagesSendMediaRequest{
		Peer:     peer,
		Media:    media,
		Message:  caption,
		Entities: entities,
		RandomID: randomId.Int64(),
	}

	resp, err := client.api.MessagesSendMedia(ctx, req)
	if err != nil {
		fmt.Println("Error", err)
	}

	upd, ok := resp.(*tg.Updates)
	if !ok {
		fmt.Println(media)
		return 0, fmt.Errorf("unexpected response type from sendMedia")
	}
	for _, update := range upd.Updates {
		switch update := update.(type) {
		case *tg.UpdateMessageID:
			return update.ID, nil
		}
	}
	return 0, fmt.Errorf("could not find new message ID in updates (media)")
}

func (client *Client) sendMultiMedia(ctx context.Context, peer tg.InputPeerClass, medias []MultiMediaItem) (int, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	var inputMedias []tg.InputSingleMedia
	for i, item := range medias {
		maxInt := new(big.Int).Lsh(big.NewInt(1), 63)
		randomId, err := rand.Int(rand.Reader, maxInt)
		if err != nil {
			return 0, errors.Wrap(err, "RandomId")
		}

		im := tg.InputSingleMedia{
			Media:    item.Media,
			RandomID: randomId.Int64(),
		}
		if i == 0 {
			im.Message = item.Caption
			im.Entities = item.Entities
		}
		inputMedias = append(inputMedias, im)
	}

	req := &tg.MessagesSendMultiMediaRequest{
		Peer:       peer,
		MultiMedia: inputMedias,
	}
	resp, err := client.api.MessagesSendMultiMedia(ctx, req)
	if err != nil {
		fmt.Println("Error", err)
		return 0, errors.Wrap(err, "MessagesSendMultiMedia")
	}
	upd, ok := resp.(*tg.Updates)
	if !ok {
		return 0, fmt.Errorf("unexpected response type from sendMultiMedia")
	}
	for _, update := range upd.Updates {
		switch update := update.(type) {
		case *tg.UpdateMessageID:
			return update.ID, nil
		}
	}
	return 0, fmt.Errorf("could not find new message ID in updates (multiMedia)")
}

func (client *Client) pinMessage(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	req := &tg.MessagesUpdatePinnedMessageRequest{
		Peer:   peer,
		ID:     msgID,
		Silent: false,
		Unpin:  false,
	}
	_, err := client.api.MessagesUpdatePinnedMessage(ctx, req)
	return err
}

func sanitizePhone(phone string) string {
	var out []rune
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			out = append(out, r)
		}
	}
	return string(out)
}
