package botmanager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"telego-bot-api/internal/converter"
)

// storedBusinessConnection keeps MTProto routing metadata out of Bot API
// responses while persisting it across process restarts.
type storedBusinessConnection struct {
	converter.BusinessConnection
	DCID int `json:"_dc_id,omitempty"`
}

func (b *BotInstance) cacheBusinessConnection(ctx context.Context, connection *converter.BusinessConnection, dcID int) {
	if connection == nil || connection.ID == "" {
		return
	}
	b.busConnsMu.Lock()
	if b.busConns == nil {
		b.busConns = make(map[string]*converter.BusinessConnection)
	}
	if b.busConnDCs == nil {
		b.busConnDCs = make(map[string]int)
	}
	b.busConns[connection.ID] = connection
	if dcID > 0 {
		b.busConnDCs[connection.ID] = dcID
	} else {
		dcID = b.busConnDCs[connection.ID]
	}
	b.busConnsMu.Unlock()

	if b.redisStore != nil {
		record := storedBusinessConnection{BusinessConnection: *connection, DCID: dcID}
		if data, err := json.Marshal(record); err == nil {
			_ = b.redisStore.SaveBusinessConnection(ctx, connection.ID, data)
		}
	}
}

func (b *BotInstance) cachedBusinessConnection(connectionID string, requireDC bool) (*converter.BusinessConnection, int, bool) {
	b.busConnsMu.RLock()
	connection := b.busConns[connectionID]
	dcID := b.busConnDCs[connectionID]
	b.busConnsMu.RUnlock()
	return connection, dcID, connection != nil && (!requireDC || dcID > 0)
}

func (b *BotInstance) businessConnection(ctx context.Context, connectionID string, requireDC bool) (*converter.BusinessConnection, int, error) {
	if connection, dcID, ok := b.cachedBusinessConnection(connectionID, requireDC); ok {
		return connection, dcID, nil
	}

	if b.redisStore != nil {
		if data, err := b.redisStore.GetBusinessConnection(ctx, connectionID); err == nil && len(data) > 0 {
			var record storedBusinessConnection
			if err := json.Unmarshal(data, &record); err == nil && record.ID != "" {
				connection := record.BusinessConnection
				b.cacheBusinessConnection(ctx, &connection, record.DCID)
				if !requireDC || record.DCID > 0 {
					return &connection, record.DCID, nil
				}
			}
		}
	}

	if b.raw == nil {
		return nil, 0, fmt.Errorf("business connection %q routing information is unavailable", connectionID)
	}
	updates, err := b.raw.AccountGetBotBusinessConnection(ctx, connectionID)
	if err != nil {
		return nil, 0, fmt.Errorf("get business connection %q: %w", connectionID, err)
	}

	var rawConnection *tg.BotBusinessConnection
	var users []tg.UserClass
	switch u := updates.(type) {
	case *tg.Updates:
		users = u.Users
		for _, update := range u.Updates {
			if value, ok := update.(*tg.UpdateBotBusinessConnect); ok {
				rawConnection = &value.Connection
				break
			}
		}
	case *tg.UpdatesCombined:
		users = u.Users
		for _, update := range u.Updates {
			if value, ok := update.(*tg.UpdateBotBusinessConnect); ok {
				rawConnection = &value.Connection
				break
			}
		}
	}
	if rawConnection == nil || rawConnection.ConnectionID != connectionID {
		return nil, 0, fmt.Errorf("business connection %q not found", connectionID)
	}
	b.peers.IngestPeers(users, nil)
	b.savePeersToRedis(users, nil)

	var user converter.User
	if entity := converter.NewEntityContext(users, nil).GetUser(rawConnection.UserID); entity != nil {
		user = *entity
	} else {
		user = converter.User{ID: rawConnection.UserID, FirstName: "User"}
	}
	connection := &converter.BusinessConnection{
		ID:         rawConnection.ConnectionID,
		User:       user,
		UserChatID: rawConnection.UserID,
		Date:       rawConnection.Date,
		CanReply:   rawConnection.Rights.Reply,
		IsEnabled:  !rawConnection.Disabled,
		Rights:     converter.ConvertBusinessBotRights(rawConnection.Rights),
	}
	b.cacheBusinessConnection(context.Background(), connection, rawConnection.DCID)
	if requireDC && rawConnection.DCID <= 0 {
		return nil, 0, fmt.Errorf("business connection %q has no datacenter routing information", connectionID)
	}
	return connection, rawConnection.DCID, nil
}

// GetBusinessConnection retrieves Bot API-visible connection information.
func (b *BotInstance) GetBusinessConnection(ctx context.Context, connectionID string) (*converter.BusinessConnection, error) {
	connection, _, err := b.businessConnection(ctx, connectionID, false)
	return connection, err
}

func (b *BotInstance) businessInvoker(ctx context.Context, dcID int) (telegram.CloseInvoker, error) {
	b.businessDCMu.Lock()
	defer b.businessDCMu.Unlock()
	if invoker := b.businessDCPools[dcID]; invoker != nil {
		return invoker, nil
	}
	if b.businessDCFactory == nil {
		return nil, fmt.Errorf("business datacenter factory is unavailable")
	}
	invoker, err := b.businessDCFactory(ctx, dcID)
	if err != nil {
		return nil, fmt.Errorf("connect to business datacenter %d: %w", dcID, err)
	}
	if b.businessDCPools == nil {
		b.businessDCPools = make(map[int]telegram.CloseInvoker)
	}
	b.businessDCPools[dcID] = invoker
	return invoker, nil
}

func (b *BotInstance) invokeBusiness(ctx context.Context, connectionID string, query bin.Object, output bin.Decoder) error {
	_, dcID, err := b.businessConnection(ctx, connectionID, true)
	if err != nil {
		return err
	}
	invoker, err := b.businessInvoker(ctx, dcID)
	if err != nil {
		return err
	}
	request := &tg.InvokeWithBusinessConnectionRequest{ConnectionID: connectionID, Query: query}
	invoke := businessErrorMiddleware{}.Handle(invoker)
	invoke = retryMiddleware{logger: b.logger}.Handle(invoke)
	return invoke(ctx, request, output)
}

// invokeBusinessDirect routes business-ready methods that must not be wrapped,
// such as stories.sendStory and stories.editStory, through the connection DC.
func (b *BotInstance) invokeBusinessDirect(ctx context.Context, connectionID string, query bin.Encoder, output bin.Decoder) error {
	_, dcID, err := b.businessConnection(ctx, connectionID, true)
	if err != nil {
		return err
	}
	invoker, err := b.businessInvoker(ctx, dcID)
	if err != nil {
		return err
	}
	return retryMiddleware{logger: b.logger}.Handle(invoker)(ctx, query, output)
}

func (b *BotInstance) closeBusinessInvokers() {
	b.businessDCMu.Lock()
	defer b.businessDCMu.Unlock()
	for dcID, invoker := range b.businessDCPools {
		_ = invoker.Close()
		delete(b.businessDCPools, dcID)
	}
}
