package main

import (
	cfg "github.com/conductorone/baton-ramp/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/config"
)

func main() {
	config.Generate("ramp", cfg.Config)
}
