#!/usr/bin/env python3
import json,re,sys
c=json.load(open(sys.argv[1]))
text='''server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;
    location /.well-known/acme-challenge/ { root /var/www/letsencrypt; }
    location / { return 404; }
}
server {
    listen 443 ssl default_server;
    listen [::]:443 ssl default_server;
    ssl_reject_handshake on;
}
'''
for app,domains in [('user',c['user_domains'])]+[(k,[c[k+'_domain']]) for k in ('admin','platform','agent')]:
    names=' '.join(domains)
    if app == 'user':
        names += r' ~^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.' + re.escape(c['user_domains'][0]) + '$'
    text+='''server {
    listen 80;
    listen [::]:80;
    server_name NAMES;
    location /.well-known/acme-challenge/ { root /var/www/letsencrypt; }
    location / { return 308 https://$host$request_uri; }
}
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name NAMES;
    ssl_certificate /etc/letsencrypt/live/lottery-wildcard/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/lottery-wildcard/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    root /opt/lottery/current/web/APP;
    index index.html;
    client_max_body_size 2m;
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 50s;
    }
    location /health/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
    }
    location /assets/ {
        try_files $uri =404;
        expires 7d;
    }
    location / {
        try_files $uri $uri/ /index.html;
        add_header Cache-Control "no-cache";
    }
}
'''.replace('NAMES',names).replace('APP',app)
open(sys.argv[2],'w').write(text)
