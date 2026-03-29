package main

import (
	"fmt"
	"net/http"

	"testkit/go/adapter"
	"testkit/go/domain"
	"testkit/go/patterns"
)

func main() {
	repo := adapter.NewPostgresRepo()
	svc := domain.NewService(repo)
	e, _ := svc.GetEntity("1")
	fmt.Println(e)

	strategy := patterns.NewAlphaStrategy()
	fmt.Println(strategy.Execute("input"))

	http.HandleFunc("/health", adapter.HealthHandler)
}
