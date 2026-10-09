package engine

import (
	"github.com/gin-gonic/gin"
)

// envelope is the body of every response the service sends:
//
//	{ "isSuccess": true,  "result": {...}, "error": null }
//	{ "isSuccess": false, "result": null,  "error": { "code": "...", "message": "..." } }
type envelope struct {
	IsSuccess bool       `json:"isSuccess"`
	Result    any        `json:"result"`
	Error     *errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func succeed(c *gin.Context, status int, result any) {
	c.JSON(status, envelope{IsSuccess: true, Result: result})
}

func fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, envelope{Error: &errorBody{Code: code, Message: message}})
}
