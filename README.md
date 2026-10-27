# RoomFlow API

API REST para gerenciamento de salas e reservas. **Etapa 2:** health check e CRUD de salas implementados. Reservas ainda não estão implementadas.

## Requisitos

Docker com Docker Compose; ou Go 1.27 e PostgreSQL para execução local.

## Execução com Docker Compose

Execute a migration uma vez antes de usar os endpoints de salas:

```sh
docker compose up -d --wait db
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.up.sql
docker compose up -d --build api
curl -i http://localhost:8080/healthz
```

As migrations SQL ficam em `db/migrations`, com pares numerados `.up.sql` e `.down.sql`. O número aplicado é registrado em `schema_migrations`. Elas são comandos explícitos, não executados pela API. A migration `001` deve ser aplicada uma vez; executá-la novamente produz erro. Para revertê-la, pare a API e execute:

```sh
docker compose stop api
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.down.sql
```

Com o banco disponível, o health check responde `200` e `{"data":{"status":"ok"}}`. Se o banco ficar indisponível, responde `503` e `{"error":{"code":"service_unavailable","message":"database unavailable"}}`.

## Execução local

Crie um banco PostgreSQL, execute `db/migrations/001_create_rooms.up.sql` com `psql -v ON_ERROR_STOP=1 -f` e configure as variáveis descritas em `.env.example`. O arquivo não é carregado automaticamente.

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
docker compose --profile test run --rm test
docker compose --profile test down
```

O serviço `testdb` usa `tmpfs`, sem volume persistente. O teste de persistência também reverte sua transação ao terminar. Para validar o código localmente, execute `go test ./...` e `go vet ./...`.

## Endpoints

| Método | Caminho | Finalidade |
| --- | --- | --- |
| `GET` | `/healthz` | Verificar API e banco |
| `POST` | `/rooms` | Criar sala (`201`, `Location`) |
| `GET` | `/rooms` | Listar salas (`200`) |
| `GET` | `/rooms/{id}` | Consultar sala (`200` ou `404`) |
| `PATCH` | `/rooms/{id}` | Atualizar nome e/ou capacidade (`200` ou `404`) |
| `DELETE` | `/rooms/{id}` | Excluir sala (`204` ou `404`) |
| `POST` | `/reservations` | Criar reserva |
| `GET` | `/reservations` | Listar reservas |
| `GET` | `/reservations/{id}` | Consultar reserva |
| `POST` | `/reservations/{id}/cancel` | Cancelar reserva |

`rooms` contém `id` gerado pelo PostgreSQL, `name` obrigatório e não vazio, e `capacity` positiva. JSON inválido, campos desconhecidos e dados inválidos retornam `400`; falhas inesperadas retornam `500`. Respostas de sucesso usam `{"data":...}` (exceto `204`, sem corpo) e erros usam `{"error":{"code":"...","message":"..."}}`.

Exemplos após aplicar a migration:

```sh
curl -i -X POST http://localhost:8080/rooms -H 'Content-Type: application/json' -d '{"name":"Sala Azul","capacity":8}'
curl -i http://localhost:8080/rooms
curl -i http://localhost:8080/rooms/1
curl -i -X PATCH http://localhost:8080/rooms/1 -H 'Content-Type: application/json' -d '{"capacity":10}'
curl -i -X DELETE http://localhost:8080/rooms/1
```

## Etapas e estrutura

1. **Base (concluída):** configuração, conexão PostgreSQL, health check, encerramento seguro e Compose.
2. **Salas (concluída):** migration SQL versionada, CRUD, validação e testes.
3. **Reservas:** criação, consulta, cancelamento, validação de horários e prevenção de conflitos.
4. **Finalização:** documentação e testes de integração das reservas.

`cmd/api` inicia a API; `internal/config` lê o ambiente; `internal/httpapi` contém rotas e respostas HTTP; `internal/rooms` contém o modelo e as queries explícitas; `db/migrations` contém o esquema versionado. A etapa seguinte adicionará reservas sem alterar essa estrutura básica.
