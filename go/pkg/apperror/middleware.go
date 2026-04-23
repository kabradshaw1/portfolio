package apperror

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorHandler returns Gin middleware that converts errors attached via
// c.Error() into structured JSON responses. AppError instances produce their
// own status code and error code; unknown errors become 500 INTERNAL_ERROR
// with the real message hidden from clients.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		// Take the last error (the most specific).
		err := c.Errors.Last().Err

		requestID, _ := c.Get("requestId")
		rid, _ := requestID.(string)

		var ae *AppError
		if errors.As(err, &ae) {
			if ae.HTTPStatus >= http.StatusInternalServerError {
				slog.Error("server error", "code", ae.Code, "message", ae.Message, "status", ae.HTTPStatus, "request_id", rid)
			}
			if len(ae.Fields) > 0 {
				c.JSON(ae.HTTPStatus, ValidationErrorResponse{
					Error: ValidationErrorBody{
						Code:      ae.Code,
						Message:   ae.Message,
						RequestID: rid,
						Fields:    ae.Fields,
					},
				})
				return
			}
			c.JSON(ae.HTTPStatus, ErrorResponse{
				Error: ErrorBody{
					Code:      ae.Code,
					Message:   ae.Message,
					RequestID: rid,
				},
			})
			return
		}

		// Unknown error — log the real cause, return a safe message.
		slog.Error("unhandled error", "error", err, "request_id", rid)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorBody{
				Code:      "INTERNAL_ERROR",
				Message:   "an unexpected error occurred",
				RequestID: rid,
			},
		})
	}
}
