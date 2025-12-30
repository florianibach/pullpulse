# pullpulse

TODO (fi): 
- screenshots


[![GitHub Repo](https://img.shields.io/badge/GitHub-Repository-blue?logo=github)](https://github.com/florianibach/pullpulse)

[![DockerHub Repo](https://img.shields.io/badge/Docker_Hub-Repository-blue?logo=docker)](https://hub.docker.com/r/floibach/pullpulse)

**pullpulse** is a lightweight, self-hosted watcher for **Docker Hub repository pull counts**.

---

This project is built and maintained in my free time.  
If it helps you or saves you some time, you can support my work on [![BuyMeACoffee](https://raw.githubusercontent.com/pachadotdev/buymeacoffee-badges/main/bmc-black.svg)](https://buymeacoffee.com/floibach)

Thank you for your support!

---

![Logo](https://raw.githubusercontent.com/florianibach/pullpulse/refs/heads/master/docs/logo.png)

Docker Hub only exposes a cumulative `pull_count`.  
pullpulse periodically snapshots this value and derives **deltas, rates and trends** over time – stored locally in SQLite and ready for visualization.

> No cloud, no tracking, no external dependencies.  
> Just pull counts → time series → insights.

## Features

- Track Docker Hub pull counts over time
- Automatic delta & per-hour rate calculation
- SQLite storage (single file, zero setup)
- Simple web UI to manage targets
- Docker-first & self-hosted
- Ready for Metabase / Grafana / custom analytics
- No Docker Hub login required (public repos)
- works on raspberry pi

## How it works

1. pullpulse periodically queries the Docker Hub API
2. Stores **snapshots** of `pull_count`
3. Calculates **deltas** between snapshots
4. Exposes the data via SQLite for analysis

You control:
- **What** to track (user or specific repositories)
- **How often** to poll (interval per target)

## Web UI

### Targets
Define what should be tracked:
- **Mode:**
  - `user` → all public repos of a Docker Hub user/org
  - `repos` → selected repositories only
- Polling interval per target
- Enable / disable at runtime

### Repositories
- See all discovered repositories
- Inspect pull history per repository
- View snapshot history & deltas

## Screenshots

![Targets](https://github.com/florianibach/pullpulse/blob/master/docs/screenshots/targets.png)
![Repos](https://github.com/florianibach/pullpulse/blob/master/docs/screenshots/repos.png)
![Repo Details](https://github.com/florianibach/pullpulse/blob/master/docs/screenshots/Repo%20Details.png)


## Quick start (Docker)

### Using `docker run`

```bash
docker run --rm -p 8080:8080 -v $(pwd)/data:/data floibach/pullpulse:latest
```

```yaml
services:
  pullpulse:
    image: floibach/pullpulse:latest
    container_name: pullpulse
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    restart: unless-stopped
```

Then open:[http://localhost:8080](http://localhost:8080)

## Visualization

pullpulse stores all metrics in a local SQLite database, making it easy to visualize pull history and trends.

A step-by-step guide for setting up **Metabase** with pullpulse is available in the project wiki:

- [Visualization – Metabase Setup](https://github.com/florianibach/pullpulse/wiki/Visualization-%E2%80%90-Metabase-Setup)

## Configuration

### Environment variables

| Variable          | Default              | Description             |
| ----------------- | -------------------- | ----------------------- |
| `DB_PATH`         | `/data/pulls.sqlite` | SQLite database file    |
| `LISTEN_ADDR`     | `:8080`              | Web UI bind address     |
| `HTTP_TIMEOUT`    | `15s`                | Docker Hub API timeout  |
| `USER_AGENT`      | `pullpulse/1.0`      | HTTP user agent         |
| `DOCKERHUB_TOKEN` | *(optional)*         | Token for private repos |

> Public repositories work **without authentication**.

## Using Metabase (recommended)

pullpulse stores everything in SQLite → perfect for Metabase.

Example setup:

```yaml
services:
  pullpulse:
    image: ghcr.io/YOUR_USER/pullpulse:latest
    volumes:
      - ./data:/data

  metabase:
    image: metabase/metabase
    ports:
      - "3000:3000"
    volumes:
      - ./data:/data:ro
```

Then in Metabase:

* Add **SQLite** database
* Path: `/data/pulls.sqlite`
* Start building dashboards 🚀

## Database schema (simplified)

* `targets` – what is being tracked
* `repos` – discovered repositories
* `repo_snapshots` – pull count over time
* `repo_deltas` – derived deltas & rates

Designed for **analytics first**, not OLTP.


## Rate limits & fair use

Docker Hub applies rate limiting.

**Recommended settings:**

* Interval ≥ **10–15 minutes**
* Avoid very large repo lists with short intervals

pullpulse logs API errors but keeps running.

## Disclaimer

* Not affiliated with Docker, Inc.
* Docker and Docker Hub are trademarks of Docker, Inc.
* Use responsibly and in accordance with Docker Hub Terms of Service.

This tool was developed in close collaboration with an AI chat assistant and refined iteratively through human–AI interaction.

The final design decisions, implementation, and maintenance remain entirely human-driven.


## License

MIT License
© 2025 Florian Ibach
