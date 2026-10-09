# telego-bot-api 🚀

Высокопроизводительный, легковесный сервер **Telegram Bot API**, написанный на **Go** на базе MTProto библиотеки [`gotd/td`](https://github.com/gotd/td) и хранилища **Redis**.

Полноценная кроссплатформенная (Linux, macOS, Docker) альтернатива официальному C++ серверу `telegram-bot-api`, спроектированная специально для работы под высокой нагрузкой (**десятки тысяч ботов**) с любыми фреймворками (**grammY**, **aiogram**, **Telegraf**, **python-telegram-bot**).

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
   - Гарантированная доставка обновлений (`XADD`). Long polling ждёт новых апдейтов внутри процесса и не держит соединение Redis на время `timeout`.
   - Монотонный `update_id`, сохраняющийся между перезапусками сервера.
   - Подтверждение и очистка прочитанных апдейтов по `offset` (`XDEL`).
   - Состояние MTProto (`pts`/`qts`/`seq`, ключи `telego:mtproto_state:<bot_id>` и `telego:mtproto_channels:<bot_id>`) хранится в Redis: после переподключения или перезапуска пропущенные апдейты догружаются через `updates.getDifference`. Как и в официальном Bot API, собственные исходящие сообщения бота и сообщения старше суток не доставляются.
   - Webhook: апдейты одного чата доставляются строго по порядку, неудачный апдейт задерживает только свой чат; учитывается `max_connections` (по умолчанию 40, максимум 100); апдейт, который не удалось доставить за сутки, отбрасывается.
6. **100% совместимость со спецификацией Bot API по кодам ошибок**:
   - `FLOOD_WAIT_X` → `HTTP 429 Too Many Requests` с `{"parameters": {"retry_after": X}}`.
   - `BOT_BLOCKED` / `USER_IS_BLOCKED` → `HTTP 403 Forbidden`.
   - `USER_DEACTIVATED` → `HTTP 403 Forbidden`.
   - `MIGRATE_TO_CHAT_ID` → `HTTP 400 Bad Request` с `{"parameters": {"migrate_to_chat_id": -100...}}`.
   - `409 Conflict` при вызове `getUpdates` при установленном вебхуке.
7. **Поддержка HTML, Markdown и MarkdownV2**:
   - Полноценный парсинг сущностей форматирования с автоматической коррекцией границ UTF-16 (`clampEntityBounds`).
8. **Автоматический супервизор переподключения (Auto-Reconnect)**:
   - Экспоненциальный backoff при разрывах сокета Telegram DC.
9. **Graceful Shutdown**:
   - Чистая остановка сервера (`SIGINT`/`SIGTERM`) с ожиданием in-flight HTTP-запросов и закрытием пула Redis.
10. **Промышленное структурированное логирование**:
    - Цветной консольный режим для разработки и структурированный JSON-режим для production.
    - Измерение латентности каждого HTTP-запроса.

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
├── Dockerfile                    # Минимальный Alpine multi-stage образ (~20MB)
├── docker-compose.yml            # Готовый запуск с Redis 7
├── go.mod
├── go.sum
└── README.md
```

---

## Требования

- **Go**: `1.22` или выше (для сборки из исходников).
- **Redis**: `6.2` или `7.x` (локально или удалённо).
- **Telegram App ID & Hash**: Получаются на [my.telegram.org](https://my.telegram.org).

---

## Способ 1: Универсальный запуск через Docker & Docker Compose (Рекомендуется)

Самый быстрый способ развернуть `telego-bot-api` на **любой ОС** (Ubuntu, Debian, CentOS, macOS, Windows) без необходимости ставить Go и Redis на хост.

```bash
# 1. Клонируйте репозиторий
git clone https://github.com/aleksfolt/telego-bot-api.git
cd telego-bot-api

# 2. Задайте свои ключи Telegram (или используйте дефолтные) и запустите контейнеры
export TELEGO_API_ID=2040
export TELEGO_API_HASH="b18441a1ff607e10a989891a5462e627"

docker compose up -d --build
```

Просмотр логов:
```bash
docker compose logs -f telego-bot-api
```

Остановка:
```bash
docker compose down
```

---

## Способ 2: Сборка из исходников (Linux и macOS)

### 1. Установка Redis

- **Ubuntu / Debian**:
  ```bash
  sudo apt update && sudo apt install -y redis-server
  sudo systemctl enable --now redis-server
  ```

- **CentOS / RHEL / AlmaLinux / Rocky Linux / Fedora**:
  ```bash
  sudo dnf install -y redis
  sudo systemctl enable --now redis
  ```

- **Arch Linux**:
  ```bash
  sudo pacman -S redis
  sudo systemctl enable --now redis
  ```

- **macOS**:
  ```bash
  brew install redis
  brew services start redis
  ```

- **Через Docker (если не хотите ставить пакет в систему)**:
  ```bash
  docker run -d --name redis -p 6379:6379 --restart unless-stopped redis:7-alpine
  ```

---

### 2. Сборка бинарника

```bash
git clone https://github.com/aleksfolt/telego-bot-api.git
cd telego-bot-api

# Сборка оптимизированного бинарника под текущую систему:
go build -ldflags="-s -w" -o telego-server ./cmd/server
```

> **Совет по кросс-компиляции**: вы можете собрать бинарник под Linux прямо с macOS или Windows:
> ```bash
> # Linux AMD64 (стандартные VPS/серверы)
> CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o telego-server-linux-amd64 ./cmd/server
>
> # Linux ARM64 (Apple Silicon, Raspberry Pi, ARM VPS)
> CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o telego-server-linux-arm64 ./cmd/server
> ```

---

### 3. Запуск шлюза

Вы можете передавать параметры через флаги командной строки либо через переменные окружения.

**Через флаги CLI**:
```bash
./telego-server \
  --api-id=2040 \
  --api-hash="b18441a1ff607e10a989891a5462e627" \
  --http-addr=0.0.0.0:8082 \
  --redis-addr=127.0.0.1:6379 \
  --log-level=info \
  --log-format=console
```

**Через переменные окружения**:
```bash
export TELEGO_API_ID=2040
export TELEGO_API_HASH="b18441a1ff607e10a989891a5462e627"
export TELEGO_HTTP_ADDR="0.0.0.0:8082"
export TELEGO_REDIS_ADDR="127.0.0.1:6379"
export TELEGO_LOG_LEVEL="info"
export TELEGO_LOG_FORMAT="console"

./telego-server
```

---

## Способ 3: Автозапуск в Linux через Systemd (Production 24/7)

Для серверов под управлением Linux рекомендуется настроить службу `systemd` с автоперезапуском при сбоях.

1. Скопируйте скомпилированный бинарник:
   ```bash
   sudo mkdir -p /opt/telego-bot-api
   sudo cp telego-server /opt/telego-bot-api/
   sudo chmod +x /opt/telego-bot-api/telego-server
   ```

2. Создайте файл сервиса `/etc/systemd/system/telego-bot-api.service`:
   ```ini
   [Unit]
   Description=Telego Telegram Bot API Gateway
   After=network.target redis.service redis-server.service
   Wants=network.target

   [Service]
   Type=simple
   Restart=always
   RestartSec=5
   LimitNOFILE=65535
   WorkingDirectory=/opt/telego-bot-api
   ExecStart=/opt/telego-bot-api/telego-server \
     --api-id=2040 \
     --api-hash=b18441a1ff607e10a989891a5462e627 \
     --http-addr=0.0.0.0:8082 \
     --redis-addr=127.0.0.1:6379 \
     --log-format=json \
     --log-level=info

   [Install]
   WantedBy=multi-user.target
   ```

3. Активируйте и запустите службу:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now telego-bot-api
   ```

4. Проверка статуса и логов:
   ```bash
   sudo systemctl status telego-bot-api
   sudo journalctl -u telego-bot-api -f
   ```

---

## Таблица параметров конфигурации

| Флаг CLI | Переменная окружения | По умолчанию | Описание |
|---|---|---|---|
| `--api-id` | `TELEGO_API_ID` / `TELEGRAM_API_ID` | `0` | Telegram API ID с my.telegram.org |
| `--api-hash` | `TELEGO_API_HASH` / `TELEGRAM_API_HASH` | `""` | Telegram API Hash с my.telegram.org |
| `--http-addr` | `TELEGO_HTTP_ADDR` / `HTTP_ADDR` | `127.0.0.1:8081` | Адрес и порт HTTP сервера Bot API (для доступа извне укажите `0.0.0.0:<порт>`) |
| `--http-read-timeout` | `TELEGO_HTTP_READ_TIMEOUT` | `30m` | Максимальное время чтения запроса (включая загрузку файла) |
| `--stall-dump-dir` | `TELEGO_STALL_DUMP_DIR` | `$TMPDIR/telego-stalls` | Папка для автоматических снимков горутин при зависании (запрос к Telegram или очередь updates дольше 30 с). Пустое значение отключает |
| `--public-status` | `TELEGO_PUBLIC_STATUS` | `false` | Отдавать `/health`, `/status`, `/metrics` публичным IP (по умолчанию только localhost и приватные сети) |
| `--redis-addr` | `TELEGO_REDIS_ADDR` / `REDIS_ADDR` | `127.0.0.1:6379` | Адрес Redis (хост:порт) |
| `--redis-pass` | `TELEGO_REDIS_PASSWORD` / `REDIS_PASSWORD` | `""` | Пароль Redis (если включен AUTH) |
| `--redis-db` | `TELEGO_REDIS_DB` / `REDIS_DB` | `0` | Номер базы Redis |
| `--log-level` | `TELEGO_LOG_LEVEL` / `LOG_LEVEL` | `info` | Уровень логов (`debug`, `info`, `warn`, `error`) |
| `--log-format` | `TELEGO_LOG_FORMAT` / `LOG_FORMAT` | `console` | Формат логов: `console` (цветной текст) или `json` (для production/ELK) |
| `--log-sample-first` | `TELEGO_LOG_SAMPLE_FIRST` | `5` | Сколько построчных логов (updates, доставки webhook, успешные API-запросы) бота за секунду пишется полностью |
| `--log-sample-thereafter` | `TELEGO_LOG_SAMPLE_THEREAFTER` | `100` | Дальше пишется каждая N-я такая строка бота; `1` отключает сэмплирование |
| `--mtproto-debug` | `TELEGO_MTPROTO_DEBUG` | `false` | Подробный дамп кадров MTProto wire transport |
| `--outbound-ips` | `TELEGO_OUTBOUND_IPS` | `""` | Список локальных исходящих IP через запятую |

---

## Подключение вашего бота

Шлюз полностью совместим с любыми Telegram Bot API библиотеками. Просто укажите URL шлюза в качестве базового адреса API.

### grammY (TypeScript / Node.js)
```typescript
import { Bot } from "grammy";

const bot = new Bot("<BOT_TOKEN>", {
  client: {
    apiRoot: "http://127.0.0.1:8082",
  },
});

bot.command("start", (ctx) => ctx.reply("Привет через telego-bot-api!"));
bot.start();
```

### aiogram 3.x (Python)
```python
from aiogram import Bot, Dispatcher
from aiogram.client.session.aiohttp import AiohttpSession
from aiogram.client.telegram import TelegramAPIServer

session = AiohttpSession(api=TelegramAPIServer.from_base("http://127.0.0.1:8082"))
bot = Bot(token="<BOT_TOKEN>", session=session)
dp = Dispatcher()

# bot handlers...
```

### Telegraf (JavaScript)
```javascript
const { Telegraf } = require("telegraf");

const bot = new Telegraf("<BOT_TOKEN>", {
  telegram: {
    apiRoot: "http://127.0.0.1:8082",
  },
});

bot.start((ctx) => ctx.reply("Привет!"));
bot.launch();
```

---

## Диагностика и статус

Шлюз предоставляет встроенный эндпоинт состояния:

- `GET /health` или `GET /status`:
  Возвращает состояние Redis и очереди вебхуков, число ботов и время последней активности каждого бота:
  ```json
  {
    "ok": true,
    "status": "healthy",
    "redis": "connected",
    "total_bots": 4,
    "active_bots": 4,
    "hibernated_bots": 0,
    "webhook_queue": 0,
    "bots": [
      {
        "bot_id": 7026718333,
        "username": "example_bot",
        "hibernated": false,
        "idle_seconds": 12,
        "last_active": "2026-09-24T12:00:00Z"
      }
    ]
  }
  ```

Построчные логи (`Update received`, `Webhook delivered successfully`, успешные `API Request`) сэмплируются отдельно для каждого бота: первые 5 строк в секунду пишутся полностью, дальше каждая сотая, а в записанной строке поле `sampled_out` показывает, сколько пропущено. Боты с небольшим трафиком логируются целиком; у загруженного бота поиск по `user_id` находит не все события. Предупреждения и ошибки не сэмплируются. Логи MTProto (`mtproto.*`) содержат `bot_id` и `dc_id`.

При `--log-format=json` входящие обновления содержат `user_id`, если у события есть отправитель, а ошибки методов отправки содержат `chat_id`. Для поиска событий и ошибок конкретного пользователя в логах systemd:

```bash
USER_ID=123456789
sudo journalctl -u telego-bot-api --since "24 hours ago" -o cat --no-pager |
  jq -Rrc --argjson id "$USER_ID" 'fromjson? | select(.user_id == $id or .chat_id == $id)'
```

### Зависшие MTProto-соединения

Синхронизация updates (`updates.getDifference`, `getChannelDifference`, `getState`) ограничена 60 секундами. Updates от соединения сначала попадают в буфер, поэтому зависший gap-менеджер не блокирует соединение и оно может переподключиться. Watchdog пересоздаёт MTProto-клиента бота, если бот с трафиком (не меньше 60 updates за 10 минут) 4 минуты не получает updates и при этом синхронизация зависла. Перезапуск не чаще раза в 10 минут, в логе: `MTProto client watchdog`. Скачивание файлов `/file/...`: не больше 8 одновременно на бота (иначе `429`), скачивание без прогресса 60 секунд обрывается (`504`).

Метрики на `/metrics`:
- `telego_bot_updates_received_total{bot_id}` — updates, полученные ботом от Telegram;
- `telego_bot_seconds_since_last_update{bot_id}` — секунд с последнего update;
- `telego_bot_watchdog_restarts_total{bot_id}` — перезапуски клиента watchdog'ом.

Пример алерта: бот с заметным трафиком 5 минут не получает updates.

```
telego_bot_seconds_since_last_update > 300
  and on(bot_id) rate(telego_bot_updates_received_total[30m] offset 5m) > 0.1
```

`Webhook delivered successfully` означает HTTP-ответ `2xx` от приложения бота. Обработка обновления и отправка ответа происходят в приложении, поэтому его ошибки нужно искать в логах самого приложения. Доставленные обновления удаляются из Redis Stream и позже недоступны для поиска по ID.

---

## Лицензия

[MIT License](LICENSE)
