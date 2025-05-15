#!/bin/bash

# Define project directory, project name, and services
PROJECT_DIR="$(dirname "$(realpath "$0")")"
PROJECT_NAME="hll-geofences-midcap"
SERVICES=("hll-geofences-midcap" "hll-geofences-lastcap")
LOG_FILE="$PROJECT_DIR/restart-containers.log"

# Function to log messages with timestamp
log_message() {
  echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" >> "$LOG_FILE"
}

# Function to check if a container is running
is_container_running() {
  local service=$1
  local container_id
  container_id=$(docker ps -q -f name="^${service}$")
  if [ -n "$container_id" ]; then
    return 0 # Running
  else
    return 1 # Not running
  fi
}

# Function to start a service
start_service() {
  local service=$1
  log_message "Starting service: $service"
  cd "$PROJECT_DIR" || { log_message "Failed to change to $PROJECT_DIR"; exit 1; }
  if docker compose -p "$PROJECT_NAME" up -d "$service" >> "$LOG_FILE" 2>&1; then
    log_message "Successfully started $service"
  else
    log_message "Failed to start $service"
    exit 1
  fi
}

# Function to stop a service
stop_service() {
  local service=$1
  log_message "Stopping service: $service"
  cd "$PROJECT_DIR" || { log_message "Failed to change to $PROJECT_DIR"; exit 1; }
  if docker compose -p "$PROJECT_NAME" stop "$service" >> "$LOG_FILE" 2>&1; then
    log_message "Successfully stopped $service"
  else
    log_message "Failed to stop $service"
    exit 1
  fi
}

# Function to restart a service
restart_service() {
  local service=$1
  if is_container_running "$service"; then
    log_message "Container $service is running. Restarting it."
    stop_service "$service"
    sleep 2
    start_service "$service"
  else
    log_message "Container $service is not running. Starting it."
    start_service "$service"
  fi
}

# Main script logic
case "$1" in
  start)
    for service in "${SERVICES[@]}"; do
      start_service "$service"
    done
    ;;
  stop)
    for service in "${SERVICES[@]}"; do
      stop_service "$service"
    done
    ;;
  restart)
    for service in "${SERVICES[@]}"; do
      restart_service "$service"
    done
    ;;
  *)
    echo "Usage: $0 {start|stop|restart}"
    exit 1
    ;;
esac
