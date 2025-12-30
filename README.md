# Precious Time Tracker

A simple, efficient, and lightweight time tracking application built with **Go**, **HTMX**, and **SQLite**.

![Precious Time Tracker](https://img.shields.io/badge/Status-Active-brightgreen)
![Go Version](https://img.shields.io/badge/Go-1.25.5-blue)
![CI/CD](https://github.com/alessandrocuzzocrea/precious-time-tracker/actions/workflows/ci-cd.yaml/badge.svg)

## Features

- **Real-time Tracking**: A persistent timer bar at the top of the screen shows your current activity.
- **Tag Support**: Organize your time entries using hashtags (e.g., `#work`, `#client-a`).
- **Full History**: View your past time entries in a clean, paginated table.
- **Easy Editing**: Inline editing for descriptions and times using HTMX.
- **No Login Required**: Designed for personal use with a local SQLite database.
- **Mobile Friendly**: Responsive UI built with custom CSS.

## Tech Stack

- **Backend**: [Go](https://golang.org/) (Golang)
- **Database**: [SQLite](https://www.sqlite.org/) (Pure Go driver, no CGO)
- **Frontend**: [HTMX](https://htmx.org/) for dynamic interactions
- **Migrations**: [Goose](https://github.com/pressly/goose)
- **SQL Boilerplate**: [sqlc](https://sqlc.dev/)
- **Live Reload**: [Air](https://github.com/cosmtrek/air)

## Getting Started

### Prerequisites

- [Go](https://golang.org/dl/) (version 1.25.5 or later)
- [Air](https://github.com/cosmtrek/air) (optional, for development)

### Running Locally

1. Clone the repository:

   ```bash
   git clone https://github.com/alessandrocuzzocrea/precious-time-tracker.git
   cd precious-time-tracker
   ```

2. Start the application with live reload (recommended):

   ```bash
   air
   ```

   Alternatively, run it directly:

   ```bash
   go run ./cmd/server/main.go
   ```

3. Open your browser and navigate to `http://localhost:8080`.

## Build and Deployment

### Docker

You can run the application using Docker:

```bash
docker build -t precious-time-tracker .
docker run -p 8080:8080 precious-time-tracker
```

### GitHub Actions

The project includes a CI/CD pipeline in `.github/workflows/ci-cd.yaml` that handles testing and builds on every push to the `main` branch.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
