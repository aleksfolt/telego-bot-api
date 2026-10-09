package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/gotd/td/telegram/updates"
	"github.com/redis/go-redis/v9"
)

// UpdatesStateStorage persists the MTProto updates state (pts/qts/date/seq) of each bot
// and the pts of its channels, so that after a reconnect or a restart the gap manager
// fetches the updates Telegram sent meanwhile via updates.getDifference.
type UpdatesStateStorage struct {
	client *redis.Client
}

var _ updates.StateStorage = (*UpdatesStateStorage)(nil)

// UpdatesStateStorage returns the Redis-backed updates.StateStorage.
func (r *RedisStore) UpdatesStateStorage() *UpdatesStateStorage {
	return &UpdatesStateStorage{client: r.client}
}

func updatesStateKey(botID int64) string {
	return fmt.Sprintf("telego:mtproto_state:%d", botID)
}

func updatesChannelsKey(botID int64) string {
	return fmt.Sprintf("telego:mtproto_channels:%d", botID)
}

// setExistingFields updates fields of an existing state hash only: StateStorage must
// fail when the state was never initialized.
var setExistingFields = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
	return redis.error_reply('updates state not found')
end
return redis.call('HSET', KEYS[1], unpack(ARGV))
`)

func (s *UpdatesStateStorage) setFields(ctx context.Context, botID int64, fields ...any) error {
	return setExistingFields.Run(ctx, s.client, []string{updatesStateKey(botID)}, fields...).Err()
}

// GetState returns the stored state of the bot.
func (s *UpdatesStateStorage) GetState(ctx context.Context, botID int64) (updates.State, bool, error) {
	values, err := s.client.HGetAll(ctx, updatesStateKey(botID)).Result()
	if err != nil {
		return updates.State{}, false, err
	}
	if len(values) == 0 {
		return updates.State{}, false, nil
	}
	var state updates.State
	for field, dst := range map[string]*int{"pts": &state.Pts, "qts": &state.Qts, "date": &state.Date, "seq": &state.Seq} {
		if *dst, err = strconv.Atoi(values[field]); err != nil {
			return updates.State{}, false, fmt.Errorf("parse %s: %w", field, err)
		}
	}
	return state, true, nil
}

// SetState replaces the state and forgets channel states, like the gotd in-memory storage.
func (s *UpdatesStateStorage) SetState(ctx context.Context, botID int64, state updates.State) error {
	_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, updatesStateKey(botID), "pts", state.Pts, "qts", state.Qts, "date", state.Date, "seq", state.Seq)
		pipe.Del(ctx, updatesChannelsKey(botID))
		return nil
	})
	return err
}

func (s *UpdatesStateStorage) SetPts(ctx context.Context, botID int64, pts int) error {
	return s.setFields(ctx, botID, "pts", pts)
}

func (s *UpdatesStateStorage) SetQts(ctx context.Context, botID int64, qts int) error {
	return s.setFields(ctx, botID, "qts", qts)
}

func (s *UpdatesStateStorage) SetDate(ctx context.Context, botID int64, date int) error {
	return s.setFields(ctx, botID, "date", date)
}

func (s *UpdatesStateStorage) SetSeq(ctx context.Context, botID int64, seq int) error {
	return s.setFields(ctx, botID, "seq", seq)
}

func (s *UpdatesStateStorage) SetDateSeq(ctx context.Context, botID int64, date, seq int) error {
	return s.setFields(ctx, botID, "date", date, "seq", seq)
}

// GetChannelPts returns the stored pts of a channel.
func (s *UpdatesStateStorage) GetChannelPts(ctx context.Context, botID, channelID int64) (int, bool, error) {
	pts, err := s.client.HGet(ctx, updatesChannelsKey(botID), strconv.FormatInt(channelID, 10)).Int()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return pts, true, nil
}

// SetChannelPts stores the pts of a channel.
func (s *UpdatesStateStorage) SetChannelPts(ctx context.Context, botID, channelID int64, pts int) error {
	return s.client.HSet(ctx, updatesChannelsKey(botID), strconv.FormatInt(channelID, 10), pts).Err()
}

// ForEachChannels calls f for every channel with a stored pts.
func (s *UpdatesStateStorage) ForEachChannels(ctx context.Context, botID int64, f func(ctx context.Context, channelID int64, pts int) error) error {
	values, err := s.client.HGetAll(ctx, updatesChannelsKey(botID)).Result()
	if err != nil {
		return err
	}
	for field, value := range values {
		channelID, err := strconv.ParseInt(field, 10, 64)
		if err != nil {
			continue
		}
		pts, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		if err := f(ctx, channelID, pts); err != nil {
			return err
		}
	}
	return nil
}
