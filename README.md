# Recharging Electric Cars

Este projeto simula um sistema de recarga de carros elétricos, composto por três componentes principais: **Servidor Central**, **Ponto de Recarga** e **Carro**. Cada componente se comunica via sockets TCP e UDP para gerenciar notificações de bateria, reservas de pontos de recarga, confirmações de chegada e sessões de carregamento.

---

## Estrutura do Projeto

- **Servidor Central**: Gerencia conexões de carros e pontos de recarga.
  - Local: `cloud/src/server.go`
- **Ponto de Recarga**: Simula um ponto de recarga que aceita reservas e gerencia sessões de carregamento.
  - Local: `recharge-point/src/main.go`
- **Carro**: Simula um carro que interage com o servidor para encontrar pontos de recarga e gerenciar sessões.
  - Local: `car/src/main.go`

---

## Pré-requisitos

- **Go**: Certifique-se de ter o Go instalado (versão 1.18 ou superior).
- **Portas Disponíveis**:
  - Servidor Central:
    - Porta `8080` para pontos de recarga (TCP).
    - Porta `8081` para carros (TCP).
    - Porta `8082` para atualizações de status via UDP.
- **Rede Local**: Todos os componentes devem estar na mesma rede ou configurados para comunicação remota.

---

## Como Executar

### 1. **Servidor Central**

1. Navegue até o diretório do servidor:
   ```bash
   cd cloud/src
   ```

2. Compile e execute o servidor:
   ```bash
   go run .
   ```

3. O servidor estará escutando conexões:
   - **Carros**: `localhost:8081` (TCP)
   - **Pontos de Recarga**: `localhost:8080` (TCP)
   - **Atualizações de Status**: `localhost:8082` (UDP)

---

### 2. **Ponto de Recarga**

1. Navegue até o diretório do ponto de recarga:
   ```bash
   cd recharge-point/src
   ```

2. Compile e execute o ponto de recarga:
   ```bash
   go run main.go
   ```

3. O ponto de recarga tentará se conectar ao servidor central na porta `8080` (TCP). Ele enviará seu estado inicial e aguardará mensagens do servidor. Além disso, ele enviará atualizações de status via UDP para a porta `8082`.

---

### 3. **Carro**

1. Navegue até o diretório do carro:
   ```bash
   cd car/src
   ```

2. Compile e execute o carro:
   ```bash
   go run main.go
   ```

3. O carro tentará se conectar ao servidor central na porta `8081` (TCP). Após a conexão, um menu interativo será exibido com as seguintes opções:
   - **Enviar notificação de bateria**: Envia a localização e o nível de bateria do carro.
   - **Consultar pontos de recarga disponíveis**: Solicita uma lista de pontos de recarga próximos.
   - **Solicitar reserva de ponto**: Reserva um ponto de recarga.
   - **Confirmar chegada ao ponto**: Confirma a chegada ao ponto de recarga.
   - **Encerrar sessão de carregamento**: Finaliza a sessão de carregamento e calcula o custo.
   - **Sair**: Encerra o programa.

---

### Usando Docker Compose (recomendado)

1. Certifique-se de estar na raiz do projeto onde está o arquivo `docker-compose.yml`.
2. Execute o seguinte comando:

```bash
docker-compose up --build
```

3. Para utilizar um container isolado (após o compose), digite o comando:
```bash
docker attach {id_conatainer}
```

4. Para encerar digite o comando:
```bash 
docker-compose down
```

## Comunicação via UDP

O protocolo UDP é utilizado para enviar atualizações de status dos pontos de recarga para o servidor central. Essas atualizações incluem informações como:

- ID do ponto de recarga.
- Localização (latitude e longitude).
- Disponibilidade do ponto.
- Tamanho da fila de espera.
- ID do veículo atualmente em carregamento (se aplicável).
- Nível de bateria do veículo (se aplicável).

### Como Funciona

1. **Ponto de Recarga**:
   - Periodicamente, o ponto de recarga envia mensagens via UDP para o servidor central na porta `8082`.
   - As mensagens são serializadas em JSON e contêm o estado atual do ponto de recarga.

2. **Servidor Central**:
   - O servidor escuta na porta `8082` e processa as mensagens recebidas.
   - As informações são usadas para atualizar o estado global dos pontos de recarga.

### Exemplo de Mensagem UDP

```json
{
  "id": "CP01",
  "latitude": -23.5505,
  "longitude": -46.6333,
  "available": true,
  "queue_size": 2,
  "vehicle_id": "CAR123",
  "battery": 80
}
```

---

## Fluxo de Comunicação

1. **Carro**:
   - Envia notificações de bateria e solicitações de reserva ao servidor via TCP.
   - Recebe respostas do servidor, como listas de pontos de recarga e confirmações de reserva.

2. **Servidor Central**:
   - Gerencia conexões de carros e pontos de recarga via TCP.
   - Recebe atualizações de status dos pontos de recarga via UDP.
   - Encaminha mensagens entre carros e pontos de recarga.
   - Mantém o estado dos pontos de recarga e das reservas.

3. **Ponto de Recarga**:
   - Envia atualizações de status ao servidor via UDP.
   - Recebe solicitações de reserva do servidor via TCP.
   - Simula sessões de carregamento e calcula custos.

---

## Logs e Depuração

- **Servidor Central**:
  - Exibe logs de conexões e mensagens recebidas/enviadas.
  - Processa mensagens UDP de status dos pontos de recarga.
- **Ponto de Recarga**:
  - Exibe logs detalhados de reservas, sessões de carregamento e atualizações de status.
  - Envia mensagens UDP para o servidor.
- **Carro**:
  - Exibe logs de mensagens enviadas e respostas recebidas.

---

## Personalização

- **Configuração de Portas**:
  - Altere as portas no código-fonte (`server.go`, `main.go`) para adaptá-las ao seu ambiente.
- **Localização Inicial**:
  - No ponto de recarga, as coordenadas iniciais são geradas aleatoriamente. Você pode personalizá-las em `generateRandomCoordinates`.

---

## Problemas Comuns

1. **Erro de Conexão**:
   - Certifique-se de que o servidor está em execução antes de iniciar os pontos de recarga e os carros.
   - Verifique se as portas estão disponíveis e não bloqueadas por firewall.

2. **Mensagens Não Recebidas**:
   - Verifique os logs para identificar problemas de comunicação.
   - Certifique-se de que os formatos de mensagem JSON estão corretos.

3. **UDP Não Funciona**:
   - Certifique-se de que a porta `8082` está aberta e que o servidor está escutando corretamente.
   - Verifique se o endereço IP do servidor está correto no código do ponto de recarga.

---

## Contribuição

Sinta-se à vontade para contribuir com melhorias ou novos recursos. Envie um pull request ou abra uma issue no repositório.
