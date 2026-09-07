#!/bin/bash

# install go
sudo dnf install -y golang-1.24

# install nginx
sudo dnf install -y nginx

# Install Certbot + Cloudflare Certbot plugin
sudo dnf update -y
sudo dnf install -y python3-devel augeas-devel gcc

sudo python3 -m venv /opt/certbot
sudo /opt/certbot/bin/pip install --upgrade pip
sudo /opt/certbot/bin/pip install certbot certbot-dns-cloudflare

# Configure Cloudflare credentials
sudo tee /etc/letsencrypt/cloudflare.ini > /dev/null <<'EOF'
dns_cloudflare_api_token = YOUR_CLOUDFLARE_API_TOKEN
EOF

sudo chmod 600 /etc/letsencrypt/cloudflare.ini

# Obtain wildcard SSL certificate for trybore.com
sudo /opt/certbot/bin/certbot certonly \
  --dns-cloudflare \
  --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
  --dns-cloudflare-propagation-seconds 30 \
  --cert-name trybore.com \
  -d "*.trybore.com" \
  -d "trybore.com"

sudo /opt/certbot/bin/certbot certificates

# Enable automatic Certbot renewal
sudo systemctl enable --now certbot-renew.timer
systemctl list-timers | grep certbot

# Configure Nginx reload after successful renewal
sudo mkdir -p /etc/letsencrypt/renewal-hooks/deploy

sudo tee /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh > /dev/null <<'EOF'
#!/bin/sh
systemctl reload nginx
EOF

sudo chmod +x /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh

# Test automatic renewal
sudo /opt/certbot/bin/certbot renew --dry-run

# clone repo
git clone git@github.com:Aditya-ds-1806/bore.git

# install dependencies and build
cd bore
go mod tidy
make build-server

# configure logrotate
mkdir -p /etc/logrotate.d/bore
sudo cp ../logrotate.conf /etc/logrotate.d/bore

# nginx and systemd setup
sudo cp ./nginx.conf /etc/nginx/nginx.conf
sudo cp ./bore.service /etc/systemd/system/bore.service
sudo systemctl daemon-reload
sudo systemctl enable bore
sudo systemctl enable nginx
sudo systemctl start bore
sudo systemctl start nginx
