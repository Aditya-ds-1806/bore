#!/bin/bash

# install go
sudo dnf install -y golang-1.24

# install nginx
sudo dnf install -y nginx

# install certbot
sudo dnf update -y
sudo dnf install -y epel-release
sudo dnf install -y certbot
sudo certbot certonly --manual --preferred-challenges dns -d "*.trybore.com" -d "trybore.com"

# clone repo
git clone git@github.com:Aditya-ds-1806/bore.git

# install dependencies and build
cd bore
go mod tidy
make build-server

# nginx and systemd setup
sudo cp ./nginx.conf /etc/nginx/nginx.conf
sudo cp ./bore.service /etc/systemd/system/bore.service
sudo systemctl daemon-reload
sudo systemctl enable bore
sudo systemctl enable nginx
sudo systemctl start bore
sudo systemctl start nginx
