# BudgetPulse

BudgetPulse — pet-project на Go про учёт транзакций и бюджеты по категориям.

MVP умеет:
- создавать категории
- создавать транзакции
- защищаться от дублей через Idempotency-Key
- писать событие в transactional outbox
- публиковать событие в NATS JetStream через worker
- асинхронно обновлять budget_stats
- создавать notification при первом превышении бюджета
- отдавать статус бюджета
- показывать базовые Prometheus metrics

## Стек

- Go
- net/http
- pgx
- Postgres
- NATS JetStream
- Docker Compose
- golang-migrate
- Prometheus client_golang

## Архитектура

Проект сделан как модульный монолит с двумя entrypoint'ами:

- `cmd/api` — HTTP API
- `cmd/worker` — publisher + consumers

Внутренние модули:
- `internal/ledger` — categories, transactions, idempotency, outbox write
- `internal/budget` — budgets API, budget consumer, budget_stats, notifications
- `internal/outbox` — outbox publisher
- `internal/platform` — postgres, nats, logging, metrics, config

Главный flow:

`create transaction -> outbox -> worker publish -> budget consumer -> budget_stats update -> notification`

## Основные таблицы

- `categories`
- `transactions`
- `idempotency_keys`
- `outbox_events`
- `budgets`
- `budget_stats`
- `processed_events`
- `notifications`

## Env

Создай `.env` с такими значениями:

```env
HTTP_ADDR=:8080
POSTGRES_URL=postgres://budgetpulse:budgetpulse@localhost:5432/budgetpulse?sslmode=disable
NATS_URL=nats://localhost:4222
```

## Как запустить

### 1. Поднять инфраструктуру

```bash
make compose-up
```

### 2. Миграции

```bash
migrate -path ./migrations -database "$POSTGRES_URL" up
```

Откат одной миграции:

```bash
migrate -path ./migrations -database "$POSTGRES_URL" down 1
```

### 3. Подтянуть зависимости

```bash
go mod tidy
```

### 4. Запустить API

```bash
make run-api
```

### 5. В другом терминале запустить worker

```bash
make run-worker
```

### 6. Проверить сборку

```bash
go build ./...
```

## Ручная проверка через curl

### Health

```bash
curl http://localhost:8080/healthz
```

### Создать категорию

```bash
curl -X POST http://localhost:8080/categories \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "name": "Food"
  }'
```

### Получить категории

```bash
curl "http://localhost:8080/categories?user_id=1"
```

### Создать бюджет на месяц

```bash
curl -X POST http://localhost:8080/budgets \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 1,
    "category_id": 1,
    "period_start": "2026-03-15T10:00:00Z",
    "limit_amount": "1000.00"
  }'
```

### Получить статус бюджета

```bash
curl "http://localhost:8080/budgets?user_id=1&category_id=1&period_start=2026-03-01"
```

### Создать expense transaction

```bash
curl -X POST http://localhost:8080/transactions \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: tx-0001" \
  -d '{
    "user_id": 1,
    "type": "expense",
    "amount": "250.00",
    "currency": "USD",
    "category_id": 1,
    "merchant": "Starbucks",
    "occurred_at": "2026-03-10T12:00:00Z"
  }'
```

### Повторить тот же запрос с тем же ключом

```bash
curl -X POST http://localhost:8080/transactions \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: tx-0001" \
  -d '{
    "user_id": 1,
    "type": "expense",
    "amount": "250.00",
    "currency": "USD",
    "category_id": 1,
    "merchant": "Starbucks",
    "occurred_at": "2026-03-10T12:00:00Z"
  }'
```

### Получить транзакции

```bash
curl "http://localhost:8080/transactions?user_id=1"
```

### Проверить budget status после обработки worker

```bash
curl "http://localhost:8080/budgets?user_id=1&category_id=1&period_start=2026-03-01"
```

### Проверить metrics

```bash
curl http://localhost:8080/metrics
```

## Ньюансы

### Idempotency

`CreateTransaction` требует `Idempotency-Key` в header.

Если тот же ключ приходит повторно с тем же payload, сервис возвращает сохранённый результат.  
Если тот же ключ приходит с другим payload, возвращается conflict.

### Transactional outbox

Событие не публикуется прямо из HTTP handler.  
Сначала оно сохраняется в `outbox_events` в той же DB-транзакции, что и `transactions`.

### Idempotent consumer

Consumer использует `processed_events`.  
Если `event_id` уже обработан, событие пропускается.

### Notifications

Если после обновления `budget_stats` бюджет впервые стал exceeded, создаётся запись в `notifications`.  
Одинаковые уведомления для одного месяца и категории не плодятся.
