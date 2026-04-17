.PHONY: build up down reset reset-hard reset-full logs status configs \
       configs-s2 configs-all up-all down-all reset-all reset-hard-all reset-full-all \
       scenario-tutorial scenario-standard scenario-hard add-user \
       wg-gen wg-up wg-down lint fmt fmt-check

COMPOSE_ALL = docker compose -f docker-compose.yml -f docker-compose.swarm2.yml

# Build all Docker images
build:
	$(COMPOSE_ALL) build

# Generate node configs (run before first build)
configs:
	go run ./cmd/configgen/

configs-s2:
	go run ./cmd/configgen/ --swarm 2

configs-all: configs configs-s2

# Start swarm 1 only
up:
	docker compose up -d
	@echo ""
	@echo "=== GILDED GUARDIAN — SWARM 1 ==="
	@echo "Scoreboard:     http://localhost:8080"
	@echo "Attack station: ssh operator@localhost -p 2222"
	@echo "Swarm network:  172.20.0.0/16"
	@echo ""

# Start both swarms
up-all:
	$(COMPOSE_ALL) up -d
	@echo ""
	@echo "=== GILDED GUARDIAN — BOTH SWARMS ==="
	@echo "Swarm 1 scoreboard: http://localhost:8080"
	@echo "Swarm 2 scoreboard: http://localhost:8081"
	@echo "Attack station:     ssh operator@localhost -p 2222"
	@echo "Swarm 1 network:    172.20.0.0/16"
	@echo "Swarm 2 network:    172.22.0.0/16"
	@echo ""

# Stop everything
down:
	docker compose down

down-all:
	$(COMPOSE_ALL) down

# Soft reset: restart all swarm containers
reset:
	docker compose restart controller-1 controller-2 controller-3 controller-4 controller-5 \
		sensor-1 sensor-2 sensor-3 sensor-4 sensor-5 sensor-6 \
		boomer-1 boomer-2 boomer-3 boomer-4 boomer-5 boomer-6 boomer-7 \
		boomer-8 boomer-9 boomer-10 boomer-11 boomer-12 boomer-13 boomer-14 boomer-15 \
		scenario
	@echo "Swarm 1 reset complete"

reset-all:
	$(COMPOSE_ALL) restart controller-1 controller-2 controller-3 controller-4 controller-5 \
		sensor-1 sensor-2 sensor-3 sensor-4 sensor-5 sensor-6 \
		boomer-1 boomer-2 boomer-3 boomer-4 boomer-5 boomer-6 boomer-7 \
		boomer-8 boomer-9 boomer-10 boomer-11 boomer-12 boomer-13 boomer-14 boomer-15 \
		scenario \
		s2-controller-1 s2-controller-2 s2-controller-3 s2-controller-4 s2-controller-5 \
		s2-sensor-1 s2-sensor-2 s2-sensor-3 s2-sensor-4 s2-sensor-5 s2-sensor-6 \
		s2-boomer-1 s2-boomer-2 s2-boomer-3 s2-boomer-4 s2-boomer-5 s2-boomer-6 s2-boomer-7 \
		s2-boomer-8 s2-boomer-9 s2-boomer-10 s2-boomer-11 s2-boomer-12 s2-boomer-13 s2-boomer-14 s2-boomer-15 \
		s2-scenario
	@echo "All swarms reset complete"

# Hard reset: destroy and recreate
reset-hard:
	docker compose down -v
	docker compose up -d
	@echo "Hard reset complete — fresh environment"

reset-hard-all:
	$(COMPOSE_ALL) down -v
	$(COMPOSE_ALL) up -d
	@echo "Hard reset complete — both swarms fresh"

# Full reset: regenerate configs, rebuild, deploy
reset-full:
	go run ./cmd/configgen/
	docker compose down -v
	docker compose build --no-cache
	docker compose up -d
	@echo "Full reset complete — new UUIDs, new keys"

reset-full-all:
	go run ./cmd/configgen/
	go run ./cmd/configgen/ --swarm 2
	$(COMPOSE_ALL) down -v
	$(COMPOSE_ALL) build --no-cache
	$(COMPOSE_ALL) up -d
	@echo "Full reset complete — both swarms, new UUIDs, new keys"

# Switch scenario to tutorial
scenario-tutorial:
	docker compose cp scenario/scenarios/tutorial.json scenario:/etc/mantis/scenario.json
	docker compose restart scenario
	@echo "Switched to tutorial scenario"

# Switch scenario to standard
scenario-standard:
	docker compose cp scenario/scenarios/standard.json scenario:/etc/mantis/scenario.json
	docker compose restart scenario
	@echo "Switched to standard scenario"

# Switch scenario to hard
scenario-hard:
	docker compose cp scenario/scenarios/hard.json scenario:/etc/mantis/scenario.json
	docker compose restart scenario
	@echo "Switched to hard scenario"

# Tail all logs
logs:
	docker compose logs -f --tail=50

logs-all:
	$(COMPOSE_ALL) logs -f --tail=50

# Show status
status:
	@docker compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"

status-all:
	@$(COMPOSE_ALL) ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"

# Add a team member: make add-user NAME=alice KEY="ssh-ed25519 AAAA..."
add-user:
	@if [ -z "$(NAME)" ] || [ -z "$(KEY)" ]; then \
		echo "Usage: make add-user NAME=alice KEY=\"ssh-ed25519 AAAA...\""; \
		exit 1; \
	fi
	@mkdir -p keys
	@echo "$(KEY)" > keys/$(NAME).pub
	@docker compose exec attack-station sh -c "\
		adduser -D -G team -s /bin/bash $(NAME) 2>/dev/null; \
		mkdir -p /home/$(NAME)/.ssh; \
		echo '$(KEY)' > /home/$(NAME)/.ssh/authorized_keys; \
		chmod 700 /home/$(NAME)/.ssh; \
		chmod 600 /home/$(NAME)/.ssh/authorized_keys; \
		chown -R $(NAME):team /home/$(NAME)"
	@echo "User $(NAME) added to attack station"
	@echo "Connect: ssh $(NAME)@<host-ip> -p 2222"

# --- WireGuard remote-access helpers ---
# Example: make wg-gen PEERS=4 ENDPOINT=range.example.com:51820 NAMES=dash,laura,phoenix,alice
wg-gen:
	@if [ -z "$(PEERS)" ] || [ -z "$(ENDPOINT)" ]; then \
		echo "Usage: make wg-gen PEERS=N ENDPOINT=host:port [NAMES=a,b,c]"; \
		exit 1; \
	fi
	./scripts/wg-gen.sh --peers $(PEERS) --endpoint $(ENDPOINT) $(if $(NAMES),--names $(NAMES))

wg-up:
	@test -f wireguard/server.conf || (echo "run 'make wg-gen' first" && exit 1)
	sudo wg-quick up ./wireguard/server.conf

wg-down:
	sudo wg-quick down ./wireguard/server.conf

# --- Developer hygiene ---
fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	@out=$$(gofmt -l ./cmd ./internal); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

lint:
	go vet ./...
