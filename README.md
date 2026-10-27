# RoomFlow API

API REST para gerenciamento de salas e reservas. **Etapa 3:** health check, CRUD de salas e gerenciamento de reservas implementados.

## Requisitos

Docker com Docker Compose; ou Go 1.27 e PostgreSQL para execução local.

## Execução com Docker Compose

Em um banco novo, aplique as migrations na ordem 001 → 002 antes de iniciar a API:

```sh
docker compose up -d --wait db
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.up.sql
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.up.sql
docker compose up -d --build api
curl -i http://localhost:8080/healthz
```

Se a migration 001 já estiver aplicada, execute apenas a 002. As migrations SQL ficam em `db/migrations`, com pares numerados `.up.sql` e `.down.sql`. Cada versão aplicada é registrada em `schema_migrations`. Elas são comandos explícitos, não executados pela API. Cada subida deve ser aplicada uma vez; repetir o comando produz erro. Para reverter a 002, pare a API e execute:

```sh
docker compose stop api
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.down.sql
```

Isso remove a tabela e o histórico de reservas. A extensão `btree_gist` permanece instalada porque pode ser usada por outros objetos. Para reverter também a 001, execute em seguida:

```sh
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.down.sql
```

Com o banco disponível, o health check responde `200` e `{"data":{"status":"ok"}}`. Se o banco ficar indisponível, responde `503` e `{"error":{"code":"service_unavailable","message":"database unavailable"}}`.

## Execução local

Crie um banco PostgreSQL, execute as migrations 001 e 002 em ordem com `psql -v ON_ERROR_STOP=1 -f` e configure as variáveis descritas em `.env.example`. O arquivo não é carregado automaticamente.

```sh
export DATABASE_URL='postgres://roomflow:roomflow@localhost:5432/roomflow?sslmode=disable'
export APP_PORT=8080
go run ./cmd/api
```

`DATABASE_URL` é obrigatória. `APP_PORT` usa `8080` por padrão. A aplicação verifica a conexão ao iniciar e encerra com erro se não conseguir conectar. As credenciais do exemplo servem apenas para desenvolvimento local.

## Testes

```sh
go test ./...
```

Sem `TEST_DATABASE_URL`, o teste de integração é pulado. Para executá-lo em um banco temporário e isolado com Docker Compose:

```sh
docker compose --profile test up -d --wait testdb
docker compose --profile test exec -T testdb psql -U roomflow -d roomflow_test -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.up.sql
docker compose --profile test exec -T testdb psql -U roomflow -d roomflow_test -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.up.sql
docker compose --profile test run --rm test
docker compose --profile test down
```

O serviço `testdb` usa `tmpfs`, sem volume persistente. A maioria dos testes de persistência reverte sua transação ao terminar; o teste de concorrência usa duas conexões independentes e remove os próprios dados ao final. Não use o banco de desenvolvimento como `TEST_DATABASE_URL`. Para validar o código localmente, execute `go test ./...` e `go vet ./...`.

## Endpoints

| Método | Caminho | Finalidade |
| --- | --- | --- |
| `GET` | `/healthz` | Verificar API e banco |
| `POST` | `/rooms` | Criar sala (`201`, `Location`) |
| `GET` | `/rooms` | Listar salas (`200`) |
| `GET` | `/rooms/{id}` | Consultar sala (`200` ou `404`) |
| `PATCH` | `/rooms/{id}` | Atualizar nome e/ou capacidade (`200` ou `404`) |
| `DELETE` | `/rooms/{id}` | Excluir sala (`204`, `404` ou `409` se tiver reservas) |
| `POST` | `/reservations` | Criar reserva (`201`, `Location`; `404` se a sala não existir; `409` se houver conflito) |
| `GET` | `/reservations` | Listar por início e ID (`200`) |
| `GET` | `/reservations/{id}` | Consultar reserva (`200` ou `404`) |
| `POST` | `/reservations/{id}/cancel` | Cancelar reserva (`200`, `404` ou `409` se já cancelada) |

`rooms` contém `id` gerado pelo PostgreSQL, `name` obrigatório e não vazio, e `capacity` positiva. `reservations` contém `room_id`, `starts_at`, `ends_at` e `cancelled_at`. O POST exige timestamps RFC3339 com timezone; respostas são normalizadas em UTC. O início do intervalo é inclusivo e o fim exclusivo, portanto reservas adjacentes são permitidas. O PostgreSQL usa uma constraint de exclusão GiST parcial para impedir sobreposição de reservas ativas da mesma sala, inclusive sob concorrência. Cancelar mantém o registro e libera seu intervalo. Uma sala com histórico de reservas não pode ser excluída.

JSON inválido, campos desconhecidos e dados inválidos retornam `400`; falhas inesperadas retornam `500`. Respostas de sucesso usam `{"data":...}` (exceto `204`, sem corpo) e erros usam `{"error":{"code":"...","message":"..."}}`.

Exemplos após aplicar a migration:

```sh
curl -i -X POST http://localhost:8080/rooms -H 'Content-Type: application/json' -d '{"name":"Sala Azul","capacity":8}'
curl -i http://localhost:8080/rooms
curl -i http://localhost:8080/rooms/1
curl -i -X PATCH http://localhost:8080/rooms/1 -H 'Content-Type: application/json' -d '{"capacity":10}'
curl -i -X POST http://localhost:8080/reservations -H 'Content-Type: application/json' -d '{"room_id":1,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}'
curl -i http://localhost:8080/reservations
curl -i http://localhost:8080/reservations/1
curl -i -X POST http://localhost:8080/reservations/1/cancel
curl -i -X DELETE http://localhost:8080/rooms/1
```

## Etapas e estrutura

1. **Base (concluída):** configuração, conexão PostgreSQL, health check, encerramento seguro e Compose.
2. **Salas (concluída):** migration SQL versionada, CRUD, validação e testes.
3. **Reservas (concluída):** criação, consulta, cancelamento, validação de horários e prevenção de conflitos.

`cmd/api` inicia a API; `internal/config` lê o ambiente; `internal/httpapi` contém rotas e respostas HTTP; `internal/rooms` e `internal/reservations` contêm modelos, erros de domínio e queries explícitas; `db/migrations` contém o esquema versionado.
