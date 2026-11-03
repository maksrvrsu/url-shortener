# URL Shortener (REST API на Go & PostgreSQL)

Высокопроизводительный REST API сервис для сокращения URL-адресов, написанный на **Go** с использованием реляционной СУБД **PostgreSQL**, легковесного роутера **Chi**, структурированного логгера **slog** и автоматических миграций.

Архитектура построена строго по принципам Clean Architecture с разделением на три изолированных слоя: **Handler → Service → Storage**.

---

## Быстрый старт (Запуск из коробки)

Для запуска требуется только установленный **Docker** и **Docker Compose**. Никаких дополнительных ручных настроек базы данных или миграций делать не нужно: всё поднимается и накатывается автоматически одной командой.

```bash
# 1. Клонируем репозиторий
git clone https://github.com/your-username/url-shortener.git
cd url-shortener

# 2. Копируем переменные окружения
cp .env.example .env

# 3. Поднимаем базу данных PostgreSQL и сервис
docker compose up --build
```

Сервис готов к работе и принимает запросы на `http://localhost:8082`.

---

## Архитектура слоёв

В отличие от упрощенных реализаций, где обработчики HTTP напрямую обращаются к базе данных (`handler → storage`), данный проект строго следует трехслойной модели:

```
┌──────────────────────────────────────────────┐
│             HTTP Handler Layer               │
│  - Парсинг JSON/URL параметров               │
│  - Валидация HTTP запроса                    │
│  - Установка правильных HTTP-статусов        │
│  - Рендеринг JSON ответа                     │
└──────────────────────┬───────────────────────┘
                       │ (интерфейсы UseCase)
                       ▼
┌──────────────────────────────────────────────┐
│               Service Layer                  │
│  - Бизнес-логика                             │
│  - Валидация URL схемы (http/https)          │
│  - Генерация случайного alias (6 символов)   │
│  - Обработка коллизий alias с повторами      │
│  - Маппинг ошибок хранилища в доменные       │
└──────────────────────┬───────────────────────┘
                       │ (интерфейс Storage)
                       ▼
┌──────────────────────────────────────────────┐
│               Storage Layer                  │
│  - Изолированные SQL-запросы                 │
│  - Драйвер PostgreSQL (pgx/v5)               │
│  - Автоматические миграции (golang-migrate)  │
│  - Распознавание SQLSTATE кодов (23505 и др.)│
└──────────────────────────────────────────────┘
```

1. **Handler Layer (`internal/http-server/handlers/`)**: отвечает исключительно за прием HTTP-запроса, валидацию формата данных (JSON) и формирование ответа с правильным HTTP-кодом. Не содержит SQL и бизнес-правил.
2. **Service Layer (`internal/service/`)**: сердце системы. Отвечает за генерацию случайных алиасов, повторные попытки при коллизиях, проверку протоколов URL и валидацию алиасов. **Все ручки, включая редирект, обращаются только к Service**.
3. **Storage Layer (`internal/storage/postgres/`)**: отвечает исключительно за выполнение SQL-запросов к PostgreSQL и обработку низкоуровневых ошибок БД (например, код `23505` — unique_violation преобразуется в `storage.ErrURLExists`).

---

## Конфигурация и переменные окружения

Все настройки и секреты управляются через переменные окружения (`.env`) и дублируются в `config/local.yaml`:

| Переменная | Значение по умолчанию | Описание |
|---|---|---|
| `ENV` | `local` | Режим работы (`local`, `dev`, `prod`) |
| `HTTP_SERVER_ADDRESS` | `0.0.0.0:8082` | Сетевой адрес и порт сервера |
| `HTTP_SERVER_TIMEOUT` | `4s` | Таймаут на чтение/запись запроса |
| `HTTP_SERVER_IDLE_TIMEOUT`| `60s` | Таймаут keep-alive соединений |
| `HTTP_SERVER_USER` | `admin` | Логин для Basic Auth |
| `HTTP_SERVER_PASSWORD` | `secret` | Пароль для Basic Auth |
| `POSTGRES_USER` | `postgres` | Пользователь PostgreSQL |
| `POSTGRES_PASSWORD` | `postgres` | Пароль к PostgreSQL |
| `POSTGRES_DB` | `url_shortener` | Название базы данных |
| `POSTGRES_HOST` | `postgres` (`localhost` для локального запуска без Docker) | Хост PostgreSQL |
| `POSTGRES_PORT` | `5432` | Порт PostgreSQL |
| `DATABASE_URL` | `postgres://postgres:postgres@postgres:5432/url_shortener?sslmode=disable` | Строка подключения к БД |

---

## HTTP API и примеры запросов

Авторизация: защищенные ручки (`POST /url`, `PUT /url/{alias}`, `DELETE /url/{alias}`) закрыты **Basic Auth** (`admin:secret` по умолчанию).

### 1. Создание короткого URL

- **URL:** `POST /url`
- **Auth:** Basic Auth
- **Тело запроса (с кастомным alias):**
  ```json
  {
    "url": "https://google.com",
    "alias": "my-google"
  }
  ```
- **Тело запроса (с автогенерацией alias):**
  ```json
  {
    "url": "https://google.com"
  }
  ```
- **Пример через cURL:**
  ```bash
  curl -i -u admin:secret -X POST http://localhost:8082/url \
    -H "Content-Type: application/json" \
    -d '{"url": "https://google.com", "alias": "my-google"}'
  ```
- **Ответ (`201 Created`):**
  ```json
  {
    "status": "OK",
    "alias": "my-google"
  }
  ```

### 2. Редирект по alias

- **URL:** `GET /{alias}`
- **Auth:** Публичный
- **Пример через cURL:**
  ```bash
  curl -i http://localhost:8082/my-google
  ```
- **Ответ (`302 Found`):**
  ```http
  HTTP/1.1 302 Found
  Location: https://google.com
  ```

### 3. Получение информации об URL по alias

- **URL:** `GET /url/{alias}`
- **Auth:** Публичный
- **Пример через cURL:**
  ```bash
  curl -i http://localhost:8082/url/my-google
  ```
- **Ответ (`200 OK`):**
  ```json
  {
    "status": "OK",
    "url": "https://google.com",
    "alias": "my-google"
  }
  ```

### 4. Обновление URL по alias

- **URL:** `PUT /url/{alias}`
- **Auth:** Basic Auth
- **Тело запроса:**
  ```json
  {
    "url": "https://new-url.org"
  }
  ```
- **Пример через cURL:**
  ```bash
  curl -i -u admin:secret -X PUT http://localhost:8082/url/my-google \
    -H "Content-Type: application/json" \
    -d '{"url": "https://new-url.org"}'
  ```
- **Ответ (`200 OK`):**
  ```json
  {
    "status": "OK"
  }
  ```

### 5. Удаление короткого URL

- **URL:** `DELETE /url/{alias}`
- **Auth:** Basic Auth
- **Пример через cURL:**
  ```bash
  curl -i -u admin:secret -X DELETE http://localhost:8082/url/my-google
  ```
- **Ответ (`200 OK`):**
  ```json
  {
    "status": "OK"
  }
  ```

---

## Таблица HTTP-статусов

В сервисе исключен антипаттерн «всё через 400 или 200». Перед `render.JSON(...)` всегда вызывается `render.Status(r, status)`:

| Ситуация | HTTP Код | Описание |
|---|---|---|
| Успешное создание URL | `201 Created` | Ресурс создан |
| Успешное обновление / удаление | `200 OK` | Операция выполнена |
| Редирект на целевой URL | `302 Found` | В заголовке `Location` содержится адрес |
| Некорректный/битый JSON | `400 Bad Request` | Ошибка парсинга синтаксиса JSON |
| Ошибка валидации полей | `400 Bad Request` | Невалидный URL или пустое поле |
| Не указан или неверный Basic Auth | `401 Unauthorized` | Требуется авторизация |
| Alias не найден (на редирект / get / update / delete) | `404 Not Found` | Короткая ссылка отсутствует в базе |
| Alias уже занят при создании | `409 Conflict` | Коллизия уникального идентификатора |
| Внутренняя ошибка сервера / БД | `500 Internal Server Error` | Непредвиденная ошибка на стороне бэкенда |

---

## Запуск тестов

Тесты разделены по изолированным тест-функциям для каждого тест-кейса в соответствии с чек-листом.

Креды для Basic Auth в тестах динамически подтягиваются из файла `.env` (без какого-либо хардкода).

```bash
# Запуск всех тестов проекта
go test -v ./...
```

Список тест-кейсов:
- `TestSave_Success`: успешное создание (201 Created).
- `TestSave_BadRequest`: проверка на **реально битый синтаксис JSON** (400 Bad Request).
- `TestSave_Conflict`: попытка создать уже существующий alias (409 Conflict).
- `TestRedirect_Success`: успешный редирект (302 Found + заголовок `Location`).
- `TestRedirect_NotFound`: редирект по несуществующему alias (404 Not Found).
- `TestGetURL_Success` / `TestGetURL_NotFound`: получение ссылки по alias (200 / 404).
- `TestUpdate_Success` / `TestUpdate_BadRequest` / `TestUpdate_NotFound`: обновление URL (200 / 400 / 404).
- `TestDelete_Success` / `TestDelete_NotFound`: удаление URL (200 / 404).
- `TestAuth_Unauthorized`: запрос без кредов или с неверным паролем (401 Unauthorized).
- `TestService_*`: модульные тесты сервисного слоя (генерация alias, ретраи коллизий, валидация).

---

## Структура проекта

```
url-shortener/
├── cmd/
│   └── url-shortener/
│       └── main.go                  # Точка входа, запуск сервера, graceful shutdown
├── config/
│   └── local.yaml                   # Локальный конфиг по умолчанию
├── internal/
│   ├── config/
│   │   └── config.go                # Загрузка и валидация конфигов (cleanenv + godotenv)
│   ├── http-server/
│   │   ├── handlers/
│   │   │   ├── redirect/            # Хендлер редиректа
│   │   │   └── url/
│   │   │       ├── delete/          # Хендлер удаления URL
│   │   │       ├── get/             # Хендлер получения информации об URL
│   │   │       ├── save/            # Хендлер сохранения URL
│   │   │       └── update/          # Хендлер обновления URL
│   │   └── middleware/
│   │       └── logger/              # Логирование входящих HTTP запросов через slog
│   ├── lib/
│   │   ├── api/response/            # Стандартизированный формат JSON-ответов
│   │   ├── logger/                  # Логгер и кастомные форматтеры (slogpretty, sl)
│   │   └── random/                  # Генератор случайных строк (alias)
│   ├── service/
│   │   ├── service.go               # Бизнес-логика URL shortener
│   │   └── service_test.go          # Unit-тесты сервисного слоя
│   └── storage/
│       ├── postgres/                # Слой работы с БД PostgreSQL (pgx/v5)
│       └── storage.go               # Доменные ошибки хранилища
├── migrations/
│   ├── 000001_init.up.sql           # Миграция создания таблицы url и индекса
│   ├── 000001_init.down.sql         # Миграция отката
│   └── migrations.go                # Встраивание SQL-файлов через embed.FS
├── tests/
│   └── url_shortener_test.go        # E2E и интеграционные тесты с Basic Auth из .env
├── .env.example                     # Пример всех переменных окружения
├── .gitignore                       # Игнорируемые файлы (бинарники, .env и пр.)
├── Dockerfile                       # Multi-stage сборка минимального Docker-образа
├── docker-compose.yml               # Запуск PostgreSQL и сервиса одной командой
├── go.mod                           # Go зависимости
├── go.sum                           # Хеши зависимостей
└── README.md                        # Документация проекта
```
