# Архитектура первого этапа

## Цель

Подготовить рабочий каркас Telegram Mini App и Go backend для дальнейшей поэтапной реализации бизнес-функциональности.

## Стек

- Frontend: React + TypeScript + Vite + Tailwind CSS
- Backend: Go module monolith with API, bot and worker entrypoints
- Data: PostgreSQL-ready schema and migrations directory
- Telegram: Mini App SDK integration scaffolded via frontend service wrapper

## Компоненты

### Frontend

- 5-tab mobile-first layout
- Telegram theme integration via hook
- shared design system and base UI classes
- placeholder pages for home, schedule, walks, friends, profile

### Backend

- API entrypoint under `backend/cmd/api`
- configuration loader under `backend/internal/config`
- module structure ready for further feature domains

## Далее

На втором этапе будет добавлена полноценная работа с auth, initData, PostgreSQL, миграциями, приглашениями и API конкретных доменов.
