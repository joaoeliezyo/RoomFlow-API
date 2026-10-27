# RoomFlow API

API REST para gerenciamento de salas e reservas. **Primeira etapa:** configuração, conexão com PostgreSQL e health check. Salas e reservas ainda não estão implementadas.

## Requisitos

Docker com Docker Compose; ou Go 1.27 e PostgreSQL para execução local.

## Execução com Docker Compose

```sh
docker compose up --build
curl -i http://localhost:8080/healthz
```

Com o banco disponível, o health check responde `200` e `{"data":{"status":"ok"}}`. Se o banco ficar indisponível, responde `503` e `{"error":{"code":"service_unavailable","message":"database unavailable"}}`.

## Execução local

Crie um banco PostgreSQL e configure as variáveis descritas em `.env.example`. O arquivo não é carregado automaticamente.

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

Os testes desta etapa cobrem configuração e respostas do health check sem PostgreSQL. Testes de integração serão adicionados com as operações de persistência.

## Endpoints planejados

| Método | Caminho | Finalidade |
| --- | --- | --- |
| `GET` | `/healthz` | Verificar API e banco (**implementado**) |
| `POST` | `/rooms` | Criar sala |
| `GET` | `/rooms` | Listar salas |
| `GET` | `/rooms/{id}` | Consultar sala |
| `PATCH` | `/rooms/{id}` | Atualizar sala |
| `DELETE` | `/rooms/{id}` | Excluir sala |
| `POST` | `/reservations` | Criar reserva |
| `GET` | `/reservations` | Listar reservas |
| `GET` | `/reservations/{id}` | Consultar reserva |
| `POST` | `/reservations/{id}/cancel` | Cancelar reserva |

O modelo previsto usa `rooms(id, name, capacity)` e `reservations(id, room_id, starts_at, ends_at, cancelled_at)`. Horários serão normalizados em UTC; o período inclui o início e exclui o fim. Uma restrição de exclusão parcial no PostgreSQL impedirá sobreposição de reservas ativas da mesma sala, inclusive sob concorrência. Respostas de sucesso usarão `{"data":...}` e erros usarão `{"error":{"code":"...","message":"..."}}`.

## Etapas e estrutura

1. **Base (atual):** configuração, conexão PostgreSQL, health check, encerramento seguro e Compose.
2. **Salas:** migrations SQL versionadas, CRUD, validação e testes.
3. **Reservas:** criação, consulta, cancelamento, validação de horários e prevenção de conflitos.
4. **Finalização:** testes de integração com PostgreSQL e exemplos `curl` para todos os endpoints.

`cmd/api` inicia a API; `internal/config` lê o ambiente; `internal/httpapi` contém rotas e respostas HTTP. As etapas seguintes adicionarão `internal/rooms`, `internal/reservations` e `db/migrations`, com serviços e persistência junto às regras de cada domínio.
