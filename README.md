# HLL Geofences for Advanced Seeding

This repository provides robust scripts for managing geofencing-based seeding configurations on Hell Let Loose (HLL) servers. It supports two distinct seeding modes:

- **Midcap Seeding**: Restricts gameplay to midcap objectives.
- **Last Cap Seeding**: Blocks the last two enemy lines to encourage seeding.

The solution leverages Docker for streamlined deployment and includes optional Discord bot integration for remote management, making it ideal for server administrators seeking efficient, scalable seeding control.

## Features

- **Dynamic Geofencing**: Configurable player count thresholds to trigger geofencing rules.
- **Dockerized Deployment**: Simplifies setup and ensures consistent runtime environments.
- **Discord Bot Integration**: Optional remote control via Discord for real-time management.
- **Dual Configuration Support**: Seamlessly switch between midcap and last cap seeding modes.

![Discord Docker Control](screenshot1.png)
![Advanced Seeding](screenshot2.png)

## Prerequisites

Ensure the following are installed and configured:

- **Git**: For cloning the repository.
- **Docker**: For containerized deployment.
- **Node.js and npm**: Required for the optional Discord bot.
- **HLL Server RCON Access**: Requires server IP, RCON port, and password.

## Express Installation

The express installation script automates setup, minimizing manual configuration. It’s recommended to review the `.env` file post-installation to customize warning/punishment messages or Discord settings.

1. **Clone the Repository**:
   ```bash
   git clone https://github.com/2KU77B0N3S/hll-geofences
   cd hll-geofences
   ```

2. **Execute Installation Script**:
   ```bash
   bash install_hll_geofences.sh
   ```
   This script configures environment files, Docker settings, and default seeding parameters.

3. **Review `.env` File**:
   ```bash
   nano .env
   ```
   Customize warning/punishment messages and verify Discord bot parameters (e.g., token, channel ID) if applicable.

4. **Launch Docker Container**:
   ```bash
   docker compose up -d
   ```

> **Post-Installation**: Proceed to the [Optional: Discord Bot Setup](#optional-discord-bot-setup) section for bot configuration. Adjust `seeding.midcap.yml` and `seeding.lastcap.yml` for server-specific settings (e.g., `SERVER-IP`, `RCON-PORT`, `RCON-PW`) as needed.

## Manual Installation

For granular control, follow these steps to configure the geofencing scripts manually.

1. **Clone the Repository**:
   ```bash
   git clone https://github.com/2KU77B0N3S/hll-geofences
   cd hll-geofences
   ```

2. **Configure Environment File**:
   ```bash
   mv seeding.example.env .env
   nano .env
   ```
   Populate required fields, including Discord bot token and channel ID (if used).

3. **Set Up Docker Configuration**:
   ```bash
   mv seeding.docker-compose.yml docker-compose.yml
   ```

4. **Configure Midcap Seeding**:
   ```bash
   nano seeding.midcap.yml
   ```
   Specify `SERVER-IP`, `RCON-PORT`, `RCON-PW`, and optionally adjust the player count threshold for geofencing.

5. **Configure Last Cap Seeding**:
   ```bash
   nano seeding.lastcap.yml
   ```
   Provide `SERVER-IP`, `RCON-PORT`, `RCON-PW`, and customize the player threshold as needed.

6. **Build Docker Image**:
   ```bash
   docker compose build
   ```
   Rebuild after any configuration changes to ensure consistency.

7. **Start Docker Container**:
   ```bash
   docker compose up -d
   ```

8. **Stop Docker Container** (if required):
   ```bash
   docker compose down
   ```

## Optional: Discord Bot Setup

Enable remote management by configuring the JavaScript-based Discord bot.

1. **Install Dependencies**:
   ```bash
   npm install
   ```

2. **Launch Discord Bot**:
   ```bash
   node seeding.main.mjs
   ```

> **Tip**: Use a process manager like PM2 for persistent bot execution in production environments.

## Restart Script

The `restart.sh` script simplifies management of `hll-geofences-midcap` and `hll-geofences-lastcap` containers defined in `docker-compose.yml`.

### Usage
```bash
chmod +x restart.sh
./restart.sh restart
```

### Commands
- `start`: Launches both containers.
- `stop`: Halts both containers.
- `restart`: Restarts both containers.

Logs are written to `restart-containers.log` in the root directory for troubleshooting.

## Operational Notes

- **Configuration Verification**: Always validate `.env`, `seeding.midcap.yml`, and `seeding.lastcap.yml` before launching containers.
- **Docker Rebuild**: Run `docker compose build` after modifying configuration or Docker files to apply changes.
- **Discord Bot**: Optional and can be omitted if remote control is unnecessary.
- **Persistence**: Consider PM2 or similar for long-running scripts in production.

## Managing Docker Instances

- **Check Container Status**:
   ```bash
   docker ps
   ```
- **View Logs**:
   ```bash
   docker compose logs
   ```
- **Restart Containers**:
   ```bash
   docker compose restart
   ```

## Contributing

Contributions are welcome! Submit issues, feature requests, or pull requests via the [GitHub repository](https://github.com/2KU77B0N3S/hll-geofences).
