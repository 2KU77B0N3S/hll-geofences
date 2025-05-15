#!/bin/bash

# Prompt for user inputs
read -p "Enter HLL Gameserver IP: " server_ip
read -p "Enter RCON Port: " rcon_port
read -p "Enter RCON Password: " rcon_password
read -p "Enter Midcap only Player Limit (default 50): " midcap_limit
read -p "Enter Lastcap blocked Player Limit (default 70): " lastcap_limit

# Set default values if inputs are empty
midcap_limit=${midcap_limit:-50}
lastcap_limit=${lastcap_limit:-70}

# Replace values in seeding.midcap.yml
sed -i "s/1.1.1.1/${server_ip}/g" seeding.midcap.yml
sed -i "s/123456/${rcon_port}/g" seeding.midcap.yml
sed -i "s/abcdef/${rcon_password}/g" seeding.midcap.yml
sed -i "s/50/${midcap_limit}/g" seeding.midcap.yml

# Replace values in seeding.lastcap.yml
sed -i "s/1.1.1.1/${server_ip}/g" seeding.lastcap.yml
sed -i "s/123456/${rcon_port}/g" seeding.lastcap.yml
sed -i "s/abcdef/${rcon_password}/g" seeding.lastcap.yml
sed -i "s/70/${lastcap_limit}/g" seeding.lastcap.yml

# Rename files
mv seeding.example.env .env
mv seeding.docker-compose.yml docker-compose.yml

# Build docker compose
docker compose build

echo "Installation and configuration completed!"
