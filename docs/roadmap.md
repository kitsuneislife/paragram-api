# Roadmap

Construir na ordem abaixo. Cada fase entrega algo usável e testado antes de começar a próxima.

## Fase 0 — Fundação

- [ ] Repositório Go, estrutura de pastas, Makefile
- [ ] Docker Compose: Postgres, Redis, MinIO
- [ ] Migrations (goose) + sqlc configurados
- [ ] Esqueleto do OpenAPI
- [ ] Middleware: request ID, logs, recuperação de panic, timeouts, CORS
- [ ] Formato de erro RFC 9457
- [ ] `/healthz` e `/readyz`
- [ ] Testes de integração com testcontainers rodando
- [ ] CI: build, testes, lint

## Fase 1 — MVP

- [ ] Cadastro, login, refresh token rotativo, logout
- [ ] Perfil: ver, editar, avatar
- [ ] Upload de imagem (presigned URL + worker libvips)
- [ ] Criar, ver, excluir post (com carrossel)
- [ ] Seguir e deixar de seguir (contas públicas)
- [ ] Curtir e comentar
- [ ] Feed home (fanout on read)
- [ ] Rate limit em auth
- [ ] Seed de dados fake

## Fase 2 — Social completo

- [ ] Contas privadas e pedidos de follow
- [ ] Bloquear, silenciar, restringir
- [ ] Stories (24 h, visualizações, close friends, destaques)
- [ ] Notificações in-app (agrupadas)
- [ ] Busca de usuários e hashtags
- [ ] Salvar posts e coleções
- [ ] Marcações e menções

## Fase 3 — Tempo real

- [ ] DM 1:1 e grupo via WebSocket
- [ ] Status entregue/lido, "digitando"
- [ ] Pedidos de mensagem
- [ ] Push (FCM, APNs)
- [ ] Preferências de notificação

## Fase 4 — Vídeo e descoberta

- [ ] Upload de vídeo + transcode HLS (ffmpeg)
- [ ] Reels e contagem de views
- [ ] Explorar (score por engajamento/idade)
- [ ] Sugestões de quem seguir

## Fase 5 — Segurança e conformidade

- [ ] 2FA (TOTP)
- [ ] Gerenciar sessões por dispositivo
- [ ] Denúncias, fila de moderação, endpoints de admin
- [ ] Excluir conta (soft delete + purge 30 dias) e exportar dados (LGPD)
- [ ] OAuth Google e Apple
- [ ] Métricas e tracing em produção

## Riscos conhecidos

- **Vídeo/HLS**: maior custo de processamento e armazenamento. Por isso fica na fase 4.
- **DM em tempo real**: estado de conexão, várias instâncias, entrega garantida. Por isso fica na fase 3, depois que o resto estiver estável.
- **Feed em escala**: fanout on read fica lento com muitos seguidos. Migrar para fanout on write quando as métricas mostrarem necessidade, não antes.
