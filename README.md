# telego-bot-api 🚀

Высокопроизводительный, легковесный сервер **Telegram Bot API**, написанный на **Go** на базе MTProto библиотеки [`gotd/td`](https://github.com/gotd/td) и хранилища **Redis**.

Полноценная альтернатива официальному C++ серверу `telegram-bot-api`, спроектированная специально для работы под высокой нагрузкой (**десятки тысяч ботов**) с любыми фреймворками (**grammY**, **aiogram**, **Telegraf**, **python-telegram-bot**).

---

## Ключевые преимущества перед официальным C++ сервером

1. **Потребление памяти в 5–10 раз ниже (Idle Bot Hibernation)**:
   - Автоматическая гибернация ботов, неактивных более 5 минут.
   - Освобождение L1 RAM-кэшей (пиры, медиа-ссылки, сессионные мапы).
   - Моментальное пробуждение (0 мс) при первом входящем событии или HTTP-запросе.
2. **Нативная поддержка Telegram Business**:
   - Полная совместимость с бизнес-подключениями (`business_connection_id`, `readBusinessMessage`, `deleteBusinessMessages`).
   - Автоматическая передача бизнес-контекста во все методы отправки медиа и сообщений.
3. **Потоковая загрузка медиа (Zero Disk & RAM Buffering)**:
   - Прямой стриминг медиафайлов по HTTP в MTProto без промежуточной записи на диск и без буферизации тяжелых файлов в память.
4. **Двухуровневый кэш медиафайлов (L1 RAM + L2 Redis)**:
   - Кэширование `InputPhoto` и `InputDocument` на 7 дней в Redis.
   - Повторная отправка тех же файлов происходит мгновенно без повторного аплоада в Telegram.
5. **Персистентная очередь апдейтов на Redis Streams**:
   - Гарантированная доставка обновлений (`XADD`, `XREAD BLOCK`).
   - Монотонный `update_id`, сохраняющийся между перезапусками сервера.
   - Подтверждение и очистка прочитанных апдейтов по `offset` (`XDEL`).
6. **100% совместимость со спецификацией Bot API по кодам ошибок**:
   - `FLOOD_WAIT_X` → `HTTP 429 Too Many Requests` с `{"parameters": {"retry_after": X}}`.
   - `BOT_BLOCKED` / `USER_IS_BLOCKED` → `HTTP 403 Forbidden`.
   - `USER_DEACTIVATED` → `HTTP 403 Forbidden`.
   - `MIGRATE_TO_CHAT_ID` → `HTTP 400 Bad Request` с `{"parameters": {"migrate_to_chat_id": -100...}}`.
   - `409 Conflict` при вызове `getUpdates` при установленном вебхуке.
7. **Поддержка HTML, Markdown и MarkdownV2**:
   - Полноценный парсинг сущностей форматирования с автоматической коррекцией границ UTF-16 (`clampEntityBounds`).
8. **Автоматический супервизор переподключения (Auto-Reconnect)**:
   - Экспоненциальный backoff при внезапных разрывах сетевого сокета Telegram.
9. **Graceful Shutdown**:
   - Чистая остановка сервера (`SIGINT`/`SIGTERM`) с ожиданием in-flight HTTP-запросов и закрытием пула Redis.
10. **Промышленное структурированное логирование**:
    - Цветной консольный режим для разработки и структурированный JSON-режим для production.
    - Измерение латентности каждого HTTP-запроса.
    - Фильтрация шума MTProto фреймов (ACK, ping/pong).

---

## Архитектура проекта

```
telego-bot-api/
├── cmd/
│   └── server/
│       └── main.go               # Точка входа, запуск сервера, перехват сигналов, graceful shutdown
├── internal/
│   ├── api/
│   │   ├── server.go             # FastHTTP Bot API роутер и ключевые обработчики
│   │   ├── extra_handlers.go     # Обработчики чатов, прав, стикеров, форумов, платежей
│   │   ├── error_mapper.go       # Интеллектуальный маппер ошибок MTProto -> Bot API HTTP
│   │   ├── parser.go             # Высокоскоростной биндинг JSON / multipart / urlencoded
│   │   └── api_test.go           # Интеграционные тесты API
│   ├── botmanager/
│   │   ├── bot.go                # MTProto клиент бота, супервизор реконнекта, гибернация
│   │   ├── manager.go            # Реестр ботов, восстановление из Redis, keep-alive 24/7, sweeper
│   │   ├── methods.go            # Реализация методов отправки и управления MTProto
│   │   └── mediaupload.go        # Потоковый аплоадер медиа (HTTP -> MTProto) с кэшем
│   ├── converter/
│   │   ├── models.go             # Модели Bot API (Update, Message, User, Chat, ResponseParameters)
│   │   ├── extra_models.go       # Полные модели Bot API 8.x
│   │   ├── formatting.go         # Парсинг HTML, Markdown и MarkdownV2
│   │   ├── markup.go             # Парсер инлайн-клавиатур и reply-кнопок
│   │   └── mtproto.go            # Маппер MTProto Updates -> JSON для ботов
│   ├── storage/
│   │   └── redis.go              # Redis Streams (updates), Media Cache (7d), Webhook store, Sessions
│   ├── webhook/
│   │   └── dispatcher.go         # Worker pool отправки вебхуков (fasthttp client) с ретраями
│   ├── logging/
│   │   └── logger.go             # Консольный/JSON логгер и MTProto wire-фильтр
│   ├── netpool/
│   │   └── dialer.go             # Ротация локальных исходящих IP (обход лимита портов)
│   ├── peer/
│   │   └── storage.go            # In-memory LRU кэш chat_id -> access_hash
│   └── config/
│       └── config.go             # Конфигурация флагов и ENV переменных
├── go.mod
├── go.sum
└── README.md
```

---

## Быстрый старт

### 1. Запуск Redis
```bash
brew services start redis
# или через Docker:
# docker run -d -p 6379:6379 redis:7-alpine
```

### 2. Сборка и запуск `telego-bot-api`
```bash
cd telego-bot-api
go build -o telego-server ./cmd/server

./telego-server \
  --api-id=<YOUR_APP_ID> \
  --api-hash="<YOUR_APP_HASH>" \
  --http-addr=0.0.0.0:8082 \
  --redis-addr=127.0.0.1:6379 \
  --log-level=info \
  --log-format=console
```

### 3. Подключение вашего бота (на примере grammY)
```typescript
import { Bot } from "grammy";

const bot = new Bot("<BOT_TOKEN>", {
  client: {
    apiRoot: "http://127.0.0.1:8082",
  },
});

bot.command("start", (ctx) => ctx.reply("Привет из telego-bot-api!"));
bot.start();
```

---

## Диагностика и статус

Шлюз предоставляет встроенный эндпоинт состояния:

- `GET /health` или `GET /status`:
  Возвращает количество зарегистрированных ботов, статус подключения каждого бота (active / hibernated), время последнего события и конфигурацию вебхука.

---

## Лицензия

MIT License
