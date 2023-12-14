package handler

import (
	"github.com/denizumutdereli/stream-services/internal/config"
	"github.com/denizumutdereli/stream-services/internal/service"
	"github.com/gin-gonic/gin"
)

type RestHandler struct {
	StreamService service.StreamService
	Config        *config.Config
}

type restHandlerInterface interface {
	Live(c *gin.Context)
	Read(c *gin.Context)
}

var _ restHandlerInterface = (*RestHandler)(nil)

func NewRestHandler(s service.StreamService, cfg *config.Config) *RestHandler {
	return &RestHandler{StreamService: s, Config: cfg}
}

func (s *RestHandler) Live(c *gin.Context) {

	statusCode, ok, err := s.StreamService.Live(c.Request.Context())
	if err != nil {
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(statusCode, gin.H{"status": ok})
}

func (s *RestHandler) Read(c *gin.Context) {

	statusCode, ok, err := s.StreamService.Live(c.Request.Context())
	if err != nil {
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(statusCode, gin.H{"status": ok})
}
