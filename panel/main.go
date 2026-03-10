package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type UserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func addDropbearUser(c *gin.Context) {
	var req UserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cmdStr := fmt.Sprintf("useradd -m -s /usr/sbin/nologin %s && echo '%s:%s' | chpasswd", req.Username, req.Username, req.Password)
	cmd := exec.Command("sh", "-c", cmdStr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": string(out)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "User Created successfully!", "username": req.Username})
}

func main() {
	router := gin.Default()
	router.Use(cors.Default())

	router.Static("/public", "./public")

	api := router.Group("/api")
	{
		api.POST("/users/ssh", addDropbearUser)
	}

	fmt.Println("Server running on port 8080")
	log.Fatal(router.Run(":8080"))
}
