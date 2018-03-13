LDFLAGS = -X main.version=$(VERSION)

goircd: *.go
	go build -ldflags "$(LDFLAGS)"

docker-image: *.go Dockerfile .dockerignore
	docker build -t $(shell basename $(PACKAGE)):$(VERSION) .

docker-image-push: docker-image-push-latest docker-image-push-version
	@true

docker-image-push-version: docker-image-push-latest docker-image-push-version
	docker tag  $(shell basename $(PACKAGE)):$(VERSION) $(PACKAGE):$(VERSION)
	docker push $(PACKAGE):$(VERSION)

docker-image-push-latest: docker-image
	docker tag  $(shell basename $(PACKAGE)):$(VERSION) $(PACKAGE):latest
	docker push $(PACKAGE):latest
