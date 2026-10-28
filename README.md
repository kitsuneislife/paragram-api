# Paragram

API de rede social de fotos e vídeos (estilo Instagram), para ser consumida por qualquer app.

Arquitetura, convenções e roadmap: [docs/](docs/README.md). Contrato HTTP: [api/openapi.yaml](api/openapi.yaml).

## Requisitos

- Go (versão em `go.mod`)
- Docker (Postgres, Redis e MinIO locais, e testes de integração)

## Rodar localmente

```bash
docker compose up -d
cp .env.example .env
```

Carregar o `.env` no shell:

```bash
set -a; . ./.env; set +a
```

```powershell
Get-Content .env | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { Set-Item "env:$($matches[1])" $matches[2] } }
```

Aplicar migrations e subir a API:

```bash
go run ./cmd/paragram migrate
go run ./cmd/paragram api
```

Checar: `curl localhost:8080/readyz`.

## Testes

```bash
go test ./...
```

Testes que precisam de Postgres usam testcontainers e são pulados quando o Docker não está rodando. No CI eles sempre rodam.

## Estrutura

```
cmd/paragram/         entrypoint: subcomandos api e migrate
internal/api/         HTTP: rotas, middleware, formato de resposta e erro
internal/config/      configuração via variáveis de ambiente
internal/db/          conexão Postgres e migrations
  migrations/         SQL do goose, embutido no binário
  queries/            SQL de onde o sqlc gera código (a partir da fase 1)
api/openapi.yaml      contrato HTTP, fonte da verdade
docs/                 arquitetura, convenções, regras do banco, roadmap
compose.yaml          dependências locais
```

## Variáveis de ambiente

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `DATABASE_URL` | sim | | Connection string do Postgres |
| `REDIS_URL` | sim | | Connection string do Redis |
| `PARAGRAM_ADDR` | não | `:8080` | Endereço HTTP |
| `PARAGRAM_CORS_ORIGINS` | não | vazio | Origens de navegador permitidas, separadas por vírgula |

## Commits

[Conventional Commits](https://www.conventionalcommits.org): `type(scope): resumo no imperativo`, minúsculo, até 72 caracteres. Um commit = uma mudança lógica, que compila e passa nos testes.
