cover:
	go test -v -coverprofile=profile.cov .
	go tool cover -func profile.cov
show-cover:
	go tool cover -func profile.cov
	