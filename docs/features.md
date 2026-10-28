# Funcionalidades

Tudo que a API oferece, agrupado por domínio. A fase de cada item está em [roadmap.md](roadmap.md).

## Auth e conta

- Cadastro com email ou telefone, com verificação por código.
- Login com senha (hash **argon2id**). Opcional: OAuth Google e Apple.
- **Access token** curto (15 min, JWT ou PASETO) + **refresh token** rotativo, guardado como hash no banco e revogável.
- Sessões por dispositivo: listar e encerrar.
- 2FA com TOTP e códigos de backup.
- Redefinir senha, trocar email, trocar senha.
- Desativar conta (reversível).
- Excluir conta: soft delete + remoção definitiva após 30 dias (LGPD).
- Exportar dados do usuário (LGPD).

## Perfil

- `username` único, sem diferenciar maiúsculas e minúsculas.
- Nome, bio, link, avatar.
- Conta pública ou privada.
- Contadores: posts, seguidores, seguindo (desnormalizados, ver [database.md](database.md)).
- Selo de verificado (flag, definido por admin).
- Ver perfil, grid de posts e posts em que o usuário foi marcado.

## Social graph

- Seguir e deixar de seguir.
- Conta privada: seguir vira **pedido de follow** (aceitar ou recusar).
- Listar seguidores e seguindo (paginado).
- Remover um seguidor.
- **Bloquear**: some tudo nos dois sentidos (perfil, posts, comentários, DM, busca).
- **Silenciar** (mute): esconde posts e stories sem deixar de seguir.
- **Restringir**: comentários da pessoa só ficam visíveis para ela mesma.
- Lista de **amigos próximos** (close friends).
- Sugestões de quem seguir (amigos de amigos).

## Posts

- 1 a 10 mídias por post (carrossel), legenda, localização, texto alternativo por mídia.
- Marcar usuários na foto, com posição (x, y).
- Hashtags e menções extraídas da legenda.
- Editar legenda, arquivar, excluir.
- Desligar comentários, esconder contagem de likes.
- Fixar post no perfil.

### Fluxo de upload

1. App pede upload: `POST /v1/media` → recebe `media_id` + presigned URL.
2. App sobe o arquivo direto para o storage.
3. App cria o post: `POST /v1/posts` com os `media_id`.
4. Worker processa a mídia. Status vai de `processing` para `ready` (ou `failed`).
5. Post só aparece no feed quando todas as mídias estão `ready`.

### Mídia gerada

- Tamanhos: thumb (320 px), médio (1080 px), full (2048 px) no lado maior, sem ampliar.
- Formato: JPEG. WebP/AVIF e blurhash ficam para depois.
- EXIF removido (evita vazar GPS).

## Reels / vídeo

- Upload de vídeo, transcodificado para HLS com thumbnail.
- Contagem de views no Redis, gravada no Postgres periodicamente.
- Feed de reels separado.

## Stories

- Story de imagem ou vídeo, expira em 24 h (`expires_at` + job de limpeza).
- Lista de quem viu.
- Responder story (vira DM).
- Reagir.
- Visível só para close friends (opcional por story).
- **Destaques** (highlights) fixados no perfil, não expiram.
- Bandeja de stories: perfis com stories não vistos primeiro.

## Interações

- Curtir e descurtir post. Idempotente: curtir duas vezes não duplica.
- Comentários com um nível de resposta.
- Curtir, excluir e fixar comentário.
- Salvar post, com coleções.
- Compartilhar post via DM.
- Lista de quem curtiu.

## Feed

- **Home**: posts de quem o usuário segue, do mais novo para o mais antigo.
  - Início: *fanout on read* (query com JOIN + índice `(author_id, created_at DESC)`).
  - Ao escalar: *fanout on write* (timeline pré-montada no Redis).
- **Explorar**: posts populares recentes. Score = engajamento / idade.
- Paginação sempre por cursor (ver [api-conventions.md](api-conventions.md)).

## Busca

- Usuários por `username` ou nome (trigram, tolera erro de digitação).
- Hashtags e posts de uma hashtag.
- Locais.
- Histórico de buscas recentes.

## DM / mensagens

- Conversa 1:1 e em grupo.
- Tipos: texto, mídia, post compartilhado, resposta a story.
- Status entregue e lido. Indicador "digitando".
- Tempo real via WebSocket, com fallback de polling.
- **Pedidos de mensagem**: mensagens de quem o usuário não segue ficam separadas.
- Apagar mensagem, sair do grupo, silenciar conversa.

## Notificações

- In-app: like, comentário, follow, menção, marcação, pedido de follow.
- Agrupadas ("fulano e mais 12 curtiram seu post").
- Push via FCM e APNs, com registro de token por dispositivo.
- Marcar como lida.
- Preferências por tipo de notificação.

## Moderação e segurança

- Denunciar post, comentário, usuário ou mensagem.
- Fila de moderação e endpoints de admin: banir usuário, remover conteúdo.
- Filtro de palavras em comentários, definido pelo usuário.
- Rate limit por IP e por usuário (mais rígido em login e cadastro).
- Upload: limite de tamanho, tipo validado pelo conteúdo do arquivo (não pela extensão).
- Papéis: `user`, `moderator`, `admin`.
