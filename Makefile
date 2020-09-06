DOCKER_REPO = ghcr.io/devplayer0/kubelan
DOCKER_TAG = latest

.PHONY: all clean

default: binary

docker:
	docker build -t $(DOCKER_REPO):$(DOCKER_TAG) .

push: docker
	docker push $(DOCKER_REPO):$(DOCKER_TAG)

binary:
	go build -o bin/ ./cmd/...

tools:
	@cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

dev: tools
	CompileDaemon -exclude-dir=.git -build="go build -o bin/kubelan ./cmd/kubelan" \
		-command="bin/kubelan -loglevel trace" -graceful-kill

clean:
	-rm -f bin/*
