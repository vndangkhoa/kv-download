# Forgejo CI/CD Setup Guide

This guide explains how to set up Forgejo with an integrated runner for automated Docker image builds.

## Overview

- **Forgejo** runs on your NAS at `http://100.89.182.37:3050`
- **Forgejo Runner** builds Docker images automatically when code is pushed
- **Docker Registry** stores the built images (optional)

---

## Prerequisites

- Docker and Docker Compose installed on your NAS
- SSH access to your NAS (port 2212)
- A Docker network already created: `kv-tube_default`

```bash
docker network create kv-tube_default
```

---

## Part 1: Docker Compose File

Create `docker-compose.forgejo.yml`:

```yaml
services:
  forgejo:
    image: codeberg.org/forgejo/forgejo:7.0.16
    container_name: forgejo
    environment:
      - USER_UID=1026
      - USER_GID=100
      - GITEA__database__DB_TYPE=sqlite3
      - TZ=Asia/Ho_Chi_Minh
      - GITEA__actions__ENABLED=true
      - INSTALL_LOCK=true
      - FORGEJO__server__ROOT_URL=http://100.89.182.37:3050/
    restart: always
    volumes:
      - ./forgejo-data:/data
    ports:
      - "3050:3000"
      - "2222:22"
    networks:
      - kv-tube_internal

  forgejo-runner:
    image: code.forgejo.org/forgejo/runner:6.0.1
    container_name: forgejo_runner
    restart: always
    user: "0:0"
    privileged: true
    depends_on:
      - forgejo
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./forgejo-runner-data:/data
    entrypoint:
      - sh
      - -c
      - |
        apk add --no-cache docker-cli
        if [ ! -f /data/.runner ]; then
          forgejo-runner register --no-interactive \
            --instance http://forgejo:3000 \
            --token YOUR_RUNNER_TOKEN \
            --name synology-runner \
            --labels ubuntu-latest,ubuntu-22.04,docker:host
        fi
        forgejo-runner daemon
    environment:
      - TZ=Asia/Ho_Chi_Minh
    networks:
      - kv-tube_internal

networks:
  kv-tube_internal:
    name: kv-tube_default
    external: true
```

**Notes:**
- Replace `100.89.182.37` with your NAS IP
- Replace `YOUR_RUNNER_TOKEN` with the token you generate in Forgejo UI

---

## Part 2: Start Services

### 1. Create directories on NAS

```bash
ssh -p 2212 superadmin@100.89.182.37
mkdir -p kv-tube/forgejo-data kv-tube/forgejo-runner-data
cd kv-tube
```

### 2. Copy docker-compose.forgejo.yml to NAS

### 3. Start services

```bash
docker-compose -f docker-compose.forgejo.yml up -d
```

---

## Part 3: Configure Forgejo

### 1. Initial Setup

1. Open **http://100.89.182.37:3050** in browser
2. Complete the initial setup:
   - Database: **SQLite3**
   - Administrator username and password
3. Login with your admin account

### 2. Enable Actions

Settings should already have `GITEA__actions__ENABLED=true` in the compose file.

### 3. Generate Runner Token

1. Go to **Settings** → **Actions** → **Runners**
2. Click **Generate Registration Token**
3. Copy the token and update `docker-compose.forgejo.yml`
4. Restart the runner:

```bash
docker-compose -f docker-compose.forgejo.yml restart forgejo-runner
```

### 4. Create Repository

1. Click **+** → **New Repository**
2. Name it (e.g., `my-project`)
3. Click **Create Repository**

---

## Part 4: Configure Git Remote

Add Forgejo as a remote:

```bash
git remote add forgejo http://100.89.182.37:3050/your_username/my-project.git
```

Or update existing remote:

```bash
git remote set-url forgejo http://username:TOKEN@100.89.182.37:3050/username/my-project.git
```

To generate an access token:
1. Go to **Settings** → **Applications** → **Tokens**
2. Click **Generate New Token**
3. Use the token in the remote URL

---

## Part 5: Create Workflow File

Create `.forgejo/workflows/docker-build.yml`:

```yaml
name: Build & Push Docker Image

on:
  push:
    branches: [main, master]
  workflow_dispatch:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        run: |
          apk add --no-cache git
          cd /tmp
          rm -rf my-project
          git clone http://username:TOKEN@forgejo:3000/username/my-project.git
          cd my-project
          git checkout ${GITEA_SHA:-main}

      - name: Build Docker Image
        run: |
          cd /tmp/my-project
          SHA_SHORT=$(git rev-parse --short HEAD)
          docker build --no-cache -t my-project:${SHA_SHORT} .
          docker images | grep my-project
```

**Notes:**
- Replace `username` and `TOKEN` with your Forgejo credentials
- Replace `my-project` with your project name
- This workflow builds the image but does NOT push to external registry (use manual push below)

**Why no auto push?** The runner runs inside Docker on your NAS and may have network issues reaching external registries. The image is built and saved locally - you can push manually or set up a local registry.

---

## Part 6: Push to Trigger Build

```bash
git add .forgejo/workflows/docker-build.yml
git commit -m "Add CI/CD workflow"
git push forgejo main
```

---

## Part 7: Monitor Workflow

1. Go to **http://100.89.182.37:3050/username/my-project/actions**
2. Click on the workflow run to see logs
3. Verify the image is built successfully

---

## Run the Built Image

On your NAS:

```bash
# List built images
docker images | grep my-project

# Run the container
docker run -d -p 3000:3000 -p 8080:8080 my-project:<tag>
```

---

## Push to External Registry (Manual)

If the CI/CD cannot push to your external registry (e.g., `git.khoavo.myds.me`) due to network issues, you can manually push from your NAS:

```bash
# 1. Find the built image tag
docker images | grep my-project

# 2. Tag for your registry
docker tag my-project:<commit-sha> git.khoavo.myds.me/username/my-project:<tag>

# 3. Push to registry
docker push git.khoavo.myds.me/username/my-project:<tag>
```

**Example:**
```bash
# List images
docker images | grep kv-tube

# Tag
docker tag kv-tube:abc1234 git.khoavo.myds.me/vndangkhoa/kv-tube:v11

# Push
docker push git.khoavo.myds.me/vndangkhoa/kv-tube:v11
```

---

## Optional: Set Up Docker Registry

If you want to push images to a registry:

### 1. Start Registry Container

```bash
docker run -d \
  --network kv-tube_default \
  --name registry \
  -p 5180:5000 \
  -v registry-data:/var/lib/registry \
  registry:2
```

### 2. Configure Insecure Registry (on NAS)

```bash
echo '{"insecure-registries":["172.27.0.3:5000"]}' | sudo tee /etc/docker/daemon.json
# Restart Docker daemon
```

### 3. Update Workflow to Push

```yaml
- name: Build and Push
  run: |
    cd /tmp/my-project
    SHA_SHORT=$(git rev-parse --short HEAD)
    docker build --no-cache -t my-project:${SHA_SHORT} .
    docker tag my-project:${SHA_SHORT} 172.27.0.3:5000/username/my-project:${SHA_SHORT}
    docker push 172.27.0.3:5000/username/my-project:${SHA_SHORT}
```

**Note:** Getting Docker to trust the registry requires proper TLS certificates or daemon configuration.

---

## Troubleshooting

### Runner not picking up jobs

```bash
# Check runner logs
docker logs forgejo_runner

# Restart runner
docker-compose -f docker-compose.forgejo.yml restart forgejo_runner
```

### "Could not resolve host: nas"

In workflow, use `forgejo:3000` (internal Docker network) instead of `nas:3050`

### Docker not available in runner

Ensure the runner has:
- `privileged: true` in compose
- `docker-cli` installed in entrypoint
- `/var/run/docker.sock` mounted

### Push fails with authentication error

Generate a new access token in **Settings** → **Applications** → **Tokens** and update both the git remote URL and the workflow file.

---

## File Structure

```
my-project/
├── .forgejo/
│   └── workflows/
│       └── docker-build.yml
├── docker-compose.forgejo.yml
└── (your project files)
```

---

## Useful Commands

```bash
# Start Forgejo + Runner
docker-compose -f docker-compose.forgejo.yml up -d

# View logs
docker logs forgejo
docker logs forgejo_runner

# Restart runner
docker-compose -f docker-compose.forgejo.yml restart forgejo-runner

# Stop services
docker-compose -f docker-compose.forgejo.yml down

# Check running containers
docker ps
```
