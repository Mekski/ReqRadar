# Deploying ReqRadar (24/7, free)

The goal: run the pipeline **always-on** on a free VM so Telegram alerts arrive even
when your laptop is off. The dashboard is **not** published — the API binds to
`127.0.0.1` and you reach it over an SSH tunnel.

What runs on the box: `postgres`, `nats`, `collector`, `processor`, `api`. `migrate`
and `seed` run once on boot. Everything has `restart: unless-stopped`, so a crash or a
VM reboot self-heals.

## 1. Get a free always-on VM (Oracle Cloud Always Free)

Oracle's Always Free tier includes an **Arm Ampere A1** instance (up to 4 OCPU / 24 GB
RAM) that runs forever at no cost — far more than this needs. (Any always-on Linux box
with Docker works; Oracle is the recommended free path.)

1. Create an Oracle Cloud account → **Compute → Instances → Create**.
2. Image: **Ubuntu 22.04** (Arm/Ampere shape, e.g. `VM.Standard.A1.Flex`, 1–2 OCPU / 6 GB
   is plenty). Add your SSH public key.
3. Networking: leave the default. **Do not** open port 8080 — the API stays private.
4. SSH in: `ssh ubuntu@<vm-public-ip>`.

## 2. Install Docker

```sh
sudo apt-get update && sudo apt-get install -y docker.io docker-compose-v2 git
sudo usermod -aG docker $USER && newgrp docker   # run docker without sudo
```

## 3. Get the code + secrets

```sh
git clone https://github.com/Mekski/ReqRadar.git && cd ReqRadar
cp .env.example .env
nano .env        # fill in TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID, GEMINI_API_KEY
                 # (optional: REQRADAR_API_TOKEN, GITHUB_TOKEN for backfill)
```

`.env` never leaves the box and is gitignored. The compose file points the services at
the in-cluster `postgres`/`nats` hostnames regardless of any `REQRADAR_*` DSN in `.env`.

## 4. Bring it up

```sh
docker compose -f deploy/docker-compose.prod.yml up -d --build
```

This builds the images, starts Postgres + NATS, runs `migrate` then `seed` (both
idempotent), and starts the collector/processor/api. Check it:

```sh
docker compose -f deploy/docker-compose.prod.yml ps
docker compose -f deploy/docker-compose.prod.yml logs -f api
```

## 5. Arm the firehose — once

So the first poll doesn't blast a Telegram alert for every already-open internship:

```sh
docker compose -f deploy/docker-compose.prod.yml run --rm collector /app firehose-prime
```

(Run this a single time after the first boot. It's safe to re-run — already-seen
postings are skipped.)

## 6. See the dashboard (when you want it)

The API is private. From your laptop, tunnel to it and run the web app locally against it:

```sh
ssh -L 8080:localhost:8080 ubuntu@<vm-public-ip>     # leave this open
# then, locally:
cd web && NEXT_PUBLIC_API_URL=http://localhost:8080 npm run dev   # http://localhost:3000
```

## Day-to-day ops

```sh
# update to the latest code (until CI/CD is wired):
git pull && docker compose -f deploy/docker-compose.prod.yml up -d --build

# logs / status:
docker compose -f deploy/docker-compose.prod.yml logs -f processor
docker compose -f deploy/docker-compose.prod.yml ps

# backfill 3-yr history for newly-added companies (heavy, manual; needs GITHUB_TOKEN in .env):
docker compose -f deploy/docker-compose.prod.yml run --rm collector /app backfill
```

## What's automated vs. manual

- **Automated 24/7:** polling, processing, Telegram alerts, on-add LLM enrichment
  (expected-open + ATS discovery), and self-restart on crash/reboot.
- **Manual / one-time:** VM setup + `.env`, the one-time `firehose-prime`, adding
  companies (your watchlist), and `backfill` (historical timing — can be scheduled later).
- **Coming next (CI/CD):** `git push` → GitHub Actions builds images → pushes to GHCR →
  SSH-deploys to this VM → smoke test, so updates ship automatically.
