# GoShorty domain setup

## Local name (works without buying a domain)

The default development URL is:

```text
http://goshorty.localhost:8080
```

Modern browsers resolve every `*.localhost` name to the local computer. No
hosts-file edit, DNS provider, or OneDrive integration is required.

Create `.env.local`, then start the complete stack:

```powershell
.\scripts\setup-env.ps1
docker compose --env-file .env.local up --build -d
```

Open `http://goshorty.localhost:8080`. New links use the canonical form
`http://goshorty.localhost:8080/s/<code>`. Existing
`/goshorty/<ttl>/<code>` links remain supported.

## Public Internet domain

A public name must be registered with a domain registrar; this repository
cannot reserve or purchase one automatically. After deployment, add a DNS
record at the registrar and configure these values on the hosting platform:

```dotenv
GIN_MODE=release
PUBLIC_BASE_URL=https://go.your-domain.example
ALLOWED_ORIGINS=https://go.your-domain.example
```

Recommended DNS layout:

```text
go.your-domain.example  CNAME  <hostname supplied by the hosting provider>
```

Use the platform's managed TLS certificate. Verify the deployment with:

```text
GET https://go.your-domain.example/health
GET https://go.your-domain.example/ready
GET https://go.your-domain.example/version
```

Do not point DNS at the service until `/ready` reports HTTP 200. Keep
`PUBLIC_BASE_URL` free of paths and trailing slashes.
