# Context

App components should be build/run/tested/etc using docker containers

# To build/rebuild the app

- Use `docker compose` to (re)start/build the frontend/backend
- Use `docker compose ps` to see whether or not the app is already running

# To run commands

To run commands within the context of the frontend or backend use:

`docker compose exec <frontend|backend> <command>`

Eg. use to run `npm install` for the frontend, or `alembic` on or python scripts on the backend

# Context

Look at `docker-compose.yml` to see how the project directories are mapped through to the frontend and backend containers

# Additional Rules

- DO NOT launch the webapp - always ask the user to do this for you.