package routes

import (
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WebsocketActionMessage struct {
	Action  string `json:"action"`
	Content string `json:"content"`
}

const CLIENT_BUILD_PATH = "/workspace/client/dist"

var upgrader = websocket.Upgrader{} // use default options

func BuildRoutes() *gin.Engine {
	router := gin.Default()
	// router.LoadHTMLGlob("routes/templates/*")

	router.Use(CORSMiddleware())

	router.GET("/login", Login)
	router.GET("/callback", Callback)
	router.GET("/logout", Logout)
	router.GET("/ws", Websocket)

	// Sessions
	router.POST("/session/replace", ReplaceSession)
	router.POST("/session/verify", VerifySession)

	// Tasks
	router.GET("/task", GetTasks)
	router.POST("/task", CreateTask)
	router.POST("/task/:id/retry", RetryTask)
	router.POST("/task/:id/cancel", CancelTask)
	// router.POST("/task/:id/upload_commons_template", GenerateCommonsTemplate)
	router.GET("/task/:id", GetTask)

	// Chart related info
	router.POST("/chart/parameters", GetChartParameters)

	router.Static("/assets", filepath.Join(CLIENT_BUILD_PATH, "assets"))
	// Handle SPA routing
	router.NoRoute(func(c *gin.Context) {
		// Don't handle API routes with this middleware
		// if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		// 	c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
		// 	return
		// }

		// Serve the index.html for any other route to handle SPA routing
		c.File(filepath.Join(CLIENT_BUILD_PATH, "index.html"))
	})

	return router
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, sessionId")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
