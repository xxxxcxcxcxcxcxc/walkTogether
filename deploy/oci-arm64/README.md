# OCI Always Free ARM deployment

This layout runs the Vite build as static files behind Nginx and keeps the API, long-polling Telegram bot, worker, and PostgreSQL on one Linux ARM64 VM. It uses the existing Go entry points; Docker is not required.

Oracle lists Always Free Ampere A1 capacity equivalent to 2 OCPUs and 12 GB RAM in the home region. Capacity can be unavailable, and Oracle may reclaim an instance it considers idle. Treat this as a hobby/test host, not a production availability promise. A domain name may still cost money; a free DNS hostname is an option, subject to that provider's terms. [Oracle Always Free resources](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm)

## 1. Create the VM

In the OCI Console, create an Ubuntu ARM64 Ampere A1 VM in the account's home region, within the Always Free eligible shape limits. Assign a public IPv4 address. Add ingress rules for TCP 22, 80 and 443, restricted to your own source IP for SSH.

Install PostgreSQL, Nginx, and Certbot on the VM. Create a Linux service account and folders:

```sh
sudo adduser --system --group --home /opt/walktogether walktogether
sudo install -d -o walktogether -g walktogether /opt/walktogether/backend
sudo install -d -o root -g root /var/www/walktogether
sudo install -d -o root -g root /etc/walktogether
```

Create a PostgreSQL role and database owned by that role. The API applies migrations during startup. Use a strong generated password and URL-encode reserved characters in `POSTGRES_DSN`.

## 2. Build release artifacts on the Mac

From the project root:

```sh
sh deploy/oci-arm64/build-release.sh
```

This creates Linux ARM64 binaries and a static frontend in `build/oci-arm64/`. Copy those artifacts to the VM, then install them:

```sh
sudo install -o root -g root -m 0755 build/oci-arm64/walktogether-api /usr/local/bin/
sudo install -o root -g root -m 0755 build/oci-arm64/walktogether-bot /usr/local/bin/
sudo install -o root -g root -m 0755 build/oci-arm64/walktogether-worker /usr/local/bin/
sudo cp -R build/oci-arm64/frontend/. /var/www/walktogether/
```

## 3. Configure secrets and HTTPS

Create `/etc/walktogether/walktogether.env` with the production values from the root `.env.example`. Set `APP_ENV=production`, `PORT=8080`, `TELEGRAM_BOT_MODE=polling`, the PostgreSQL DSN, the new BotFather token, bot username, Mini App URL, and the primary admin Telegram ID. Keep `VITE_API_BASE_URL` empty so API calls and the secure session cookie share the HTTPS site origin. Restrict the env file:

```sh
sudo chown root:walktogether /etc/walktogether/walktogether.env
sudo chmod 0640 /etc/walktogether/walktogether.env
```

Copy `nginx-walktogether.conf` into Nginx's site configuration and replace `YOUR_PUBLIC_HOSTNAME`. Point DNS to the VM public IP, then issue a TLS certificate with Certbot and enable the resulting HTTPS server block. Telegram Mini Apps require HTTPS. Set the same HTTPS URL in BotFather and `TELEGRAM_MINI_APP_URL`.

## 4. Start services

Install the three systemd unit files under `/etc/systemd/system/`, then run:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now walktogether-api walktogether-bot walktogether-worker
sudo systemctl status walktogether-api walktogether-bot walktogether-worker
curl -fsS https://YOUR_PUBLIC_HOSTNAME/health
```

The first API start applies migrations. Create the first administrator using the documented bootstrap SQL in the root README, then open the Main Mini App from Telegram. Keep polling enabled; this deployment does not need a webhook.

## 5. Updating and operating

Build another release, copy the API binaries and frontend assets, and restart the three services. PostgreSQL data lives in the VM database directory, not in the release folder. Back it up off-VM before upgrades. The same free VM hosts every process, so a VM stop/reclaim or regional capacity issue takes the entire app offline.
