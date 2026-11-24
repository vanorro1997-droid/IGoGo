# Запуск IGoGo через Docker

## Новая машина

Нужны Git и Docker с Compose v2 или новее. На Windows используйте Docker Desktop
с WSL2 и Linux-контейнерами. Go и Node.js для запуска не нужны.

```powershell
git clone -b pers-konstantin-v0.1 https://github.com/vanorro1997-droid/IGoGo.git
cd IGoGo
Copy-Item .env.example .env
```

На Linux/macOS вместо `Copy-Item` используйте `cp .env.example .env`.
Если `.env` уже существует, не перезаписывайте его.

В `.env` задайте свой `DB_PASSWORD` и проверьте:

```dotenv
HTTP_PORT=8080
DB_ENABLED=true
```

База нужна для регистрации, входа и истории. Для запуска и регистрации внешние
API-ключи не требуются; для реальных данных 2ГИС, погоды и Яндекса их нужно
настроить отдельно. Не добавляйте `.env` в Git.

```sh
docker compose up -d --build --wait --wait-timeout 180
docker compose ps
```

Приложение: <http://localhost:8080>. Регистрация: <http://localhost:8080/auth.html>.
На пустом томе база и таблицы создаются автоматически; существующий том сохраняется.
Если порт `5432` занят, задайте `DB_PORT=5433` в `.env`: внутри Docker порт остаётся `5432`.

## Почему образы закреплены

В основном `docker-compose.yml` и `Dockerfile` закреплены официальные образы:

| Назначение | Версия | Настройка |
| --- | --- | --- |
| База данных | PostgreSQL 15.20 / Alpine 3.23 | `POSTGRES_IMAGE` |
| Сборка приложения | Go 1.26.5 / Alpine 3.23 | `GO_IMAGE` |
| Запуск приложения | Alpine 3.23.8 | `ALPINE_IMAGE` |

По умолчанию используется Docker Official Images на Amazon ECR Public
(`public.ecr.aws/docker/library/...`). У PostgreSQL проверен тот же digest, что у
Docker Hub и Google-зеркала. Закреплены **multi-platform indexes**, а не digests
отдельных AMD64-образов. Docker автоматически выбирает подходящий Linux-образ,
в том числе для AMD64 и ARM64; принудительный `platform: linux/amd64` не нужен.

Закрепление версии, базового дистрибутива и digest исключает незаметную смену образа
из-за плавающих тегов вроде `postgres:15-alpine` и `golang:1.26.5-alpine`.
Для обычного запуска отдельный `docker-compose.override.yml` не требуется.
Старый override или `POSTGRES_IMAGE` в
окружении может переопределять основной выбор — проверьте `docker compose config --images`.

Если Amazon ECR Public недоступен, в `.env` можно указать тот же образ из Docker Hub:

```dotenv
POSTGRES_IMAGE=postgres:15.20-alpine3.23@sha256:83bdbdd781076b1543709883a93d995b53e682b078ea30e3cea012856732d1e5
GO_IMAGE=golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc
ALPINE_IMAGE=alpine:3.23.8@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2
```

`GO_IMAGE` и `ALPINE_IMAGE` передаются как аргументы сборки, не как настройки API.
При прямом `docker build` используйте соответствующие `--build-arg`.
После смены registry пересоберите приложение: `docker compose up -d --build`.

Альтернатива — `mirror.gcr.io/library/...` с теми же версиями и digests. При проверке
Google-зеркало возвращало квоту запросов к upstream; никакой публичный registry не
гарантирует доступность в любой сети. В таких случаях меняйте registry через
настройки образов, а не архитектуру или том базы.

Также допустим доверенный собственный registry с образом PostgreSQL 15.
Перед обновлением pinned-образа проверяйте актуальные security-релизы, multi-platform
manifest и запуск на нужных архитектурах. Обновляйте версию и digest вместе с тестом
`tests/docker/compose.test.cjs`; для Go/Alpine также синхронизируйте defaults в
`Dockerfile` и Compose. Смену основной версии PostgreSQL выполняйте отдельной
миграцией с резервной копией, а не простой заменой образа поверх существующего тома.

## Если возникает `exec format error`

Это ошибка запуска исполняемого файла контейнера, а не Go API. Возможны неверная
архитектура/режим Docker либо повреждение образа и локального Docker-кэша. На машине,
где воспроизводилась проблема, архитектура была AMD64, но `/bin/busybox` старого
кэшированного `postgres:15-alpine` имел нулевой размер. `pull` того же тега признал
кэш актуальным и не восстановил файл; рабочий закреплённый образ не использует этот
повреждённый вариант. Старый кэшированный Go builder тоже не выполнял `/bin/sh`:
предыдущие успешные сборки использовали кэш и не проверяли builder заново.

```sh
docker compose config --images
docker compose pull postgres
docker compose run --rm --no-deps --entrypoint postgres postgres --version
docker compose build --no-cache --pull app
docker compose up -d --wait --wait-timeout 180
docker compose logs --tail 80 postgres app
```

Если даже закреплённый образ не выполняет `postgres --version`, проверьте Linux-режим
Docker, ненужный `DOCKER_DEFAULT_PLATFORM`, состояние Docker Desktop/Engine и диска.
Повреждение хранилища Docker нельзя универсально исправить настройкой Compose.
Удаление неисправного кэша планируйте отдельно, сохраняя том базы; не используйте
`docker compose down -v`, `docker system prune --volumes` или сброс Docker с удалением данных.

Обычный `docker compose down` сохраняет базу.

## Проверка конфигурации

```sh
docker compose --env-file .env.example config --quiet
node --test tests/docker/compose.test.cjs
```

Node.js нужен только для этих тестов. Они проверяют конфигурацию и не запускают
контейнеры, не читают пользовательский `.env` и не изменяют базу.

На AMD64 проверены сборка без кэша, регистрация и вход с PostgreSQL. Для ARM64
проверены сборка без кэша и HTTP API через эмуляцию, а также `postgres --version`.
Проверка на физическом ARM-устройстве не выполнялась.

Для проверки именно сборки, а не старого кэша:

```sh
docker compose build --no-cache --pull app
docker compose up -d --wait --wait-timeout 180
```

Первую установку проверяйте на пустом Docker-кэше или этой командой, но не удаляйте
том базы ради проверки. Поддержка AMD64/ARM64 не означает поддержку Windows-контейнеров
или гарантию работы при неисправном Docker, недостатке памяти/диска либо недоступной сети.
