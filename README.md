# Telegram captcha bot on Go

Бот ограничивает новых участников группы, отправляет им простую inline-капчу и возвращает права после правильного ответа.

## Возможности

- Отслеживает новых участников в группе.
- Временно запрещает писать до прохождения проверки.
- Генерирует простую математическую капчу.
- Проверяет ответ через inline-кнопки.
- Удаляет пользователя из группы, если капча не пройдена за заданное время.
- Не хранит персональные данные на диске.

## Запуск в Docker

1. Создайте бота через [@BotFather](https://t.me/BotFather) и получите токен.
2. Добавьте бота в группу.
3. Выдайте боту права администратора:
   - Ban users / блокировка пользователей
   - Delete messages / удаление сообщений
   - Restrict members / ограничение участников
4. Отключите privacy mode у бота через BotFather, если бот не видит служебные сообщения группы.
5. Создайте файл `.env` рядом с `docker-compose.yml`:

```powershell
Copy-Item .env.example .env
```

6. Откройте `.env` и укажите реальный `BOT_TOKEN`.
7. Запустите контейнер:

```powershell
docker compose up -d --build
```

Логи:

```powershell
docker compose logs -f captcha-bot
```

Остановка:

```powershell
docker compose down
```

## Локальный запуск

```powershell
$env:BOT_TOKEN="123456:telegram-bot-token"
go run .
```

## Конфигурация

| Переменная | По умолчанию | Описание |
| --- | --- | --- |
| `BOT_TOKEN` | обязательна | Токен Telegram-бота |
| `TELEGRAM_API_ENDPOINT` | официальный API | Кастомный Bot API endpoint, формат `https://host/bot%s/%s` |
| `CAPTCHA_TIMEOUT` | `120s` | Время на прохождение капчи |
| `POLLING_TIMEOUT` | `60` | Таймаут long polling в секундах |
| `STARTUP_RETRIES` | `10` | Количество повторов подключения к Telegram при старте |
| `STARTUP_RETRY_DELAY` | `10s` | Пауза между повторами подключения |
| `KICK_ON_TIMEOUT` | `true` | Удалять пользователя при истечении таймаута |
| `LOG_LEVEL` | `info` | Зарезервировано под будущую настройку логов |

## Если контейнер не подключается к Telegram

Ошибка вида `dial tcp ...:443: i/o timeout` означает, что контейнер не может достучаться до `api.telegram.org`. Проверьте сеть Docker-хоста:

```powershell
docker run --rm alpine:3.22 wget -T 10 -S -O- https://api.telegram.org
```

Если Telegram недоступен из вашей сети, можно передать прокси через переменные окружения в `.env`:

```dotenv
HTTPS_PROXY=http://host.docker.internal:1080
HTTP_PROXY=http://host.docker.internal:1080
NO_PROXY=localhost,127.0.0.1
```

Также можно использовать собственный Telegram Bot API server и задать:

```dotenv
TELEGRAM_API_ENDPOINT=http://telegram-bot-api:8081/bot%s/%s
```

## Сборка

Проект рассчитан на Go `1.26.1`.

```powershell
go build -o autorize-bot.exe .
```

Docker-сборка использует официальный образ `golang:1.26.1-alpine`.

## Важно

Telegram позволяет ограничивать и удалять пользователей только если бот является администратором группы и имеет нужные права.
