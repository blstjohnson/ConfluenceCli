# confcli — CLI-утилита для Confluence

`confcli` — инструмент командной строки для работы с Atlassian Confluence Private Server. Позволяет получать, искать, создавать и редактировать страницы Confluence прямо из терминала. Кроссплатформенный, поддерживает множество форматов вывода, экспорт целых пространств и интеграцию с LLM-агентами.

## Установка

### Через Go

```bash
go install confcli@latest
```

### Сборка из исходников

```bash
git clone <url-репозитория>
cd confcli/confcli-bin
go build -o confcli ./cmd/confcli
```

### Установка в PATH

После сборки или скачивания бинарника:

```bash
# Установит симлинк в /usr/local/bin (может потребовать sudo)
confcli install

# Установка в произвольную директорию
confcli install --dir ~/bin

# Копирование бинарника вместо симлинка
confcli install --copy
```

### Обновление

```bash
# Проверить наличие обновлений
confcli update --check

# Обновить до последней версии
confcli update
```

## Конфигурация

### Быстрая настройка

```bash
confcli config init
confcli config set url https://confluence.example.com
confcli config set token ваш-api-токен
confcli config set username user@example.com
```

### Профили

Конфигурация хранится в `~/.confcli/config.yaml` и поддерживает несколько профилей:

```yaml
current_profile: default
profiles:
  default:
    url: "https://confluence.example.com"
    token: "ваш-api-токен"
    username: "user@example.com"
    auth_type: "bearer"
    read_only: false
  prod:
    url: "https://confluence-prod.example.com"
    token: "prod-токен"
    username: "admin@example.com"
    auth_type: "bearer"
    read_only: true
```

Поле `read_only: true` запрещает любые операции записи для профиля.

### Открытие страницы логина

```bash
confcli login
```

Откроет страницу логина Confluence в браузере по умолчанию.

## Основные команды

### Получение страниц

```bash
# По ID
confcli page get --id 123456

# По пространству и заголовку
confcli page get --space DEV --title "Архитектура проекта"

# По пути
confcli page get --path "DEV/Проект/Документация"

# Конкретная версия страницы
confcli page get --id 123456 --version 3

# С комментариями и метками
confcli page get --id 123456 --with-comments --with-labels

# Со всеми потомками (требует --format json)
confcli page get --id 123456 --with-descendants --format json
```

### Создание, обновление, удаление страниц

```bash
# Создание страницы
confcli page create --space DEV --title "Новая страница" --content-file content.md --confirm

# Создание дочерней страницы
confcli page create --space DEV --title "Подстраница" --parent 123456 --content-file body.md --confirm

# Обновление страницы
confcli page update 123456 --content-file updated.md --version-comment "Обновление" --confirm

# Удаление страницы
confcli page delete 123456 --confirm
```

Все операции записи требуют флаг `--confirm`.

### Поиск

```bash
# Простой поиск
confcli search "документация API"

# Поиск по CQL
confcli search --cql 'space = "DEV" AND text ~ "performance"' --limit 20

# Поиск в конкретном пространстве
confcli search "логирование" --space DEV
```

### Комментарии и метки

```bash
# Список комментариев
confcli page comments 123456

# Добавить комментарий
confcli page comment add 123456 --text "Нужно обновить раздел" --confirm

# Список меток
confcli page labels 123456

# Добавить метку
confcli page label add 123456 --label "важное" --confirm
```

### Сравнение версий

```bash
# Diff двух версий страницы
confcli page diff 123456 --old-version 2 --new-version 5

# Diff в формате export (чище для markdown)
confcli page diff 123456 --old-version 1 --new-version 3 --format export

# Со статистикой изменений
confcli page diff 123456 --old-version 2 --new-version 5 --summary
```

## Иерархия и экспорт пространств

### Просмотр иерархии

```bash
# Дерево пространства
confcli hierarchy space --space DEV --tree

# Плоский список всех страниц
confcli hierarchy space --space DEV --flat --format json

# Дерево поддерева по page-id
confcli hierarchy space --space DEV --page-id 123456 --tree

# Ограничение глубины
confcli hierarchy space --space DEV --depth 2 --tree
```

### Экспорт пространства на диск

```bash
# Экспорт всего пространства в директорию
confcli hierarchy space --space DEV --output-dir ./export

# Экспорт поддерева конкретной страницы
confcli hierarchy space --space DEV --page-id 123456 --output-dir ./export

# Экспорт версий страниц на определённую дату
confcli hierarchy space --space DEV --output-dir ./export --date 2024-06-15

# Экспорт в формате storage (сырой HTML)
confcli hierarchy space --space DEV --output-dir ./export --format storage
```

При экспорте отображается прогресс-бар для отслеживания хода загрузки. Ошибки загрузки отдельных страниц выводятся в stderr, но не прерывают экспорт — оставшиеся страницы продолжают скачиваться.

### Scroll Versions

Если пространство использует плагин Scroll Versions, confcli автоматически обнаружит доступные версии и выведет их список. Для экспорта конкретной версии:

```bash
# Экспорт конкретной версии
confcli hierarchy space --space DEV --output-dir ./export --scroll-version "Release 2.0"
```

### Флаги экспорта

| Флаг | Описание |
|------|----------|
| `--named-folders` | Использовать транслитерированные имена страниц для папок вместо ID |
| `--clean-names` | Заголовки страниц как имена файлов/папок: убираются запрещённые символы, точки и пробелы заменяются на `_` |
| `--flat-leaves` | Не создавать подкаталог для страниц без потомков — файл сохраняется прямо в родительскую папку |
| `--rewrite-links` | Перезаписать внутренние ссылки Confluence на относительные пути файлов (по умолчанию включено) |
| `--rewrite-tfs-links` | Перезаписать ссылки TFS/Git-репозиториев на локальные пути |
| `--save-metadata` | Сохранять `.meta.json` для каждой страницы и `_space_metadata.json` для пространства |
| `--skip-root` | Пропустить корневую страницу — дочерние страницы экспортируются прямо в output-директорию |
| `--no-toc` | Удалить оглавление из экспортированного markdown |
| `--no-length-limit` | Снять ограничение в 80 символов на длину имён файлов и папок |
| `--date` | Получить версии страниц, актуальные на указанную дату (формат: `YYYY-MM-DD` или RFC3339) |
| `--transform` | Применить профиль трансформации (имя профиля или путь к YAML-файлу) |
| `--set` | Переопределить параметры профиля трансформации (например, `--set page.format=html`) |
| `--scroll-version` | Экспортировать конкретную версию Scroll Versions (требуется плагин Scroll Versions) |

## Профили трансформации

Профили трансформации позволяют задать набор правил обработки контента при экспорте: удаление макросов, замена ссылок, модификация текста и другие преобразования. Профили описываются в формате YAML.

### Использование

```bash
# Экспорт с профилем трансформации
confcli hierarchy space --space DEV --output-dir ./export --transform my-profile

# Профиль из файла
confcli hierarchy space --space DEV --output-dir ./export --transform ./custom-profile.yaml

# Переопределение параметров профиля
confcli hierarchy space --space DEV --output-dir ./export --transform my-profile --set page.format=html --set folder.flat_leaves=true

# Получение страницы с трансформацией
confcli page get --id 123456 --transform my-profile
```

### Управление профилями

```bash
# Список доступных профилей
confcli transform list

# Показать содержимое профиля
confcli transform show my-profile

# Создать шаблон профиля
confcli transform init my-profile

# Перезаписать существующий профиль
confcli transform init my-profile --force
```

Именованные профили хранятся в `~/.confcli/transformations/` как `.yaml`-файлы. Также можно указать путь к произвольному файлу.

### Формат профиля

```yaml
folder:
  naming: slug        # slug, title или id
  length_limit: 80    # максимальная длина имён папок (0 = без ограничений)
  flat_leaves: false   # сохранять конечные страницы прямо в родительскую папку

page:
  format: markdown     # markdown, storage, html, plain, export
  strip_toc: false     # удалить оглавление
  save_metadata: false # сохранять .meta.json

  transforms:          # конвейер трансформаций контента
    - type: remove_macro
      params:
        macro_names: [toc, status]
        # preserve_content: true  # сохранить содержимое макроса (например, для expand)

    - type: remove_element
      params:
        selectors: [".confluence-information-macro"]

    - type: modify_content
      params:
        phase: post    # pre (до конвертации) или post (после конвертации)
        rules:
          - find: "старый-текст"
            replace: "новый-текст"

    - type: modify_links
      params:
        rules:
          - find: "https://old-domain.com"
            replace: "https://new-domain.com"

    - type: rewrite_tfs_links
      params:
        tfs_base_url: "https://tfs.example.com"
        local_repo_path: "/path/to/repo"

    - type: rewrite_internal_links
      params:
        conf_base_url: "https://confluence.example.com"

pages:                 # переопределения для конкретных страниц
  - id: 12345
    strip_toc: true
  - path: "*/Archive/*"
    skip: true         # пропустить страницы, соответствующие шаблону
  - path: "*/API/*"
    flatten: true      # сохранять конечные страницы прямо в родительскую папку
    skip_transforms: true  # не применять трансформации к этим страницам
```

### Типы трансформаций

| Тип | Описание |
|-----|----------|
| `remove_macro` | Удаление Confluence-макросов по имени (например, toc, status). Поддерживает `preserve_content: true` для сохранения содержимого |
| `remove_element` | Удаление HTML-элементов по CSS-селектору |
| `modify_links` | Замена URL в ссылках по правилам find/replace |
| `modify_content` | Замена текста в контенте (до или после конвертации) |
| `rewrite_tfs_links` | Перезапись ссылок TFS/Git-репозиториев на локальные пути |
| `rewrite_internal_links` | Перезапись внутренних ссылок Confluence на относительные пути |
| `expand_tiny_urls` | Раскрытие коротких ссылок Confluence (`/x/AbCd`) в полные URL перед перезаписью ссылок |

### Переопределение параметров через --set

Флаг `--set` позволяет переопределить параметры профиля без редактирования файла:

| Ключ | Описание |
|------|----------|
| `folder.naming` | Способ именования папок (slug, title, id) |
| `folder.length_limit` | Ограничение длины имён папок |
| `folder.flat_leaves` | Плоская структура для конечных страниц |
| `page.format` | Формат вывода контента |
| `page.strip_toc` | Удаление оглавления |
| `page.save_metadata` | Сохранение метаданных |

## Форматы контента

`confcli` поддерживает несколько форматов получения содержимого страниц:

| Формат | Флаг | Описание |
|--------|------|----------|
| Markdown | `--format markdown` | Конвертация из storage-формата в Markdown с поддержкой таблиц, панелей, макросов, блоков кода (по умолчанию) |
| Storage | `--format storage` | Сырой Confluence storage format (XML/HTML) — для последующего обновления страницы |
| Editor | `--format edit` | Формат редактора Confluence |
| Export | `--format export` | Export view, конвертированный в Markdown — чище для чтения, лучше для сложных таблиц |
| HTML | `--format html` | Сырой HTML |
| Plain | `--format plain` | Текст без HTML-тегов |

### Пример: получение в разных форматах

```bash
# Markdown (по умолчанию)
confcli page get --id 123456

# Storage — для редактирования и обратной загрузки
confcli page get --id 123456 --format storage

# Export — для документации и чтения
confcli page get --id 123456 --format export
```

## Форматы вывода

Глобальный флаг `--format` управляет форматом вывода метаданных:

```bash
# Текст (по умолчанию)
confcli page get --id 123456

# JSON (для автоматизации и парсинга)
confcli page get --id 123456 --format json

# YAML
confcli page get --id 123456 --format yaml
```

## Режим только для чтения

```bash
# Глобальный флаг
confcli --read-only page get --id 123456

# Или в конфигурации профиля
confcli config set read_only true
```

В этом режиме все операции записи (create, update, delete) будут отклонены.

## Автодополнение в оболочке

```bash
# Bash
confcli completion bash > /etc/bash_completion.d/confcli

# Zsh
confcli completion zsh > "${fpath[1]}/_confcli"

# Fish
confcli completion fish > ~/.config/fish/completions/confcli.fish

# PowerShell
confcli completion powershell > confcli.ps1
```

## Разработка

### Требования

- Go 1.25+
- Make
- Docker (для кроссплатформенной сборки)

### Сборка

```bash
# Сборка для текущей платформы
cd confcli-bin
go build -o confcli ./cmd/confcli

# Сборка для всех платформ через Docker
make release

# Запуск тестов
make test
```

Поддерживаемые платформы: Linux (amd64, arm64), Windows (amd64, arm64), macOS (amd64, arm64).

## Лицензия

MIT License
