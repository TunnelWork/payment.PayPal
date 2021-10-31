package paypal

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/TunnelWork/Ulysses.Lib/payment"
	"github.com/TunnelWork/payment.PayPal/internal/sqlwrapper"
	"github.com/gin-gonic/gin"
	pp "github.com/plutov/paypal/v4"
)

// For paypal.Buttons onApprove/onCancel/onError events
func (pg *PrepaidGateway) handlerPaypalExperienceOnClose(c *gin.Context) {
	OrderID := c.PostForm("order_id")
	ReferenceID := c.PostForm("ref_id")
	CaptureID := c.PostForm("capture_id")
	// Currency := c.PostForm("currency")
	// Value := c.PostForm("value")
	Action := c.PostForm("action") // approve/cancel/error

	if ReferenceID == "" || Action == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "BAD_REQUEST",
		})
		return
	}
	if (OrderID == "" || CaptureID == "") && Action == "approve" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "BAD_REQUEST",
		})
		return
	}

	switch Action {
	case "error":
		// reportTime := time.Now()
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNPAID,
				Msg:    fmt.Sprintf("(Unverified)ReferenceID %s: Paypal Button onError()", ReferenceID),
			},
		)
		c.JSON(http.StatusOK, gin.H{
			"status":  "canceled",
			"type":    "payment",
			"message": "BUYER_PAYPAL_ERROR",
			// "redirect": "", // No redirect.
		})
	case "approve":
		pg._onApprove(c, OrderID, ReferenceID, CaptureID)
	case "cancel":
		// reportTime := time.Now()
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.CLOSED,
				Msg:    fmt.Sprintf("(Unverified)ReferenceID %s: Paypal Button onCancel()", ReferenceID),
			},
		)
		c.JSON(http.StatusOK, gin.H{
			"status":  "canceled",
			"type":    "payment",
			"message": "BUYER_PAYPAL_CANCEL",
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "BAD_REQUEST",
		})
	}

}

func (pg *PrepaidGateway) _onApprove(c *gin.Context, OrderID string, ReferenceID string, CaptureID string) {
	// Fetch the OrderID's detail from PayPal:

	// Get latest Access Token
	_, err := pg.client.GetAccessToken(context.Background())
	if err != nil { // Failed to communicate with PayPal, fail.
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNKNOWN,
				Msg:    fmt.Sprintf("(Unverified)ReferenceID %s: pp.client.GetAccessToken() failed: %s", ReferenceID, err),
			},
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "SERVER_PAYPAL_BAD_AUTH",
		})
		return
	}

	// Checkout the order from PayPal
	var order *pp.Order
	order, err = pg.client.GetOrder(context.Background(), OrderID)
	if err != nil { // Failed to communicate with PayPal, fail.
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNKNOWN,
				Msg:    fmt.Sprintf("(Unverified)ReferenceID %s: pp.client.GetOrder() failed: %s", ReferenceID, err),
			},
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "SERVER_PAYPAL_BAD_ORDER",
		})
		return
	}
	// No bundle order allowed.
	if len(order.PurchaseUnits) != 1 {
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNKNOWN,
				Msg:    fmt.Sprintf("(Unverified)ReferenceID %s: pp.Order includes multiple PurchaseUnits left unhandled", ReferenceID),
			},
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "SERVER_PAYPAL_BAD_ORDER",
		})
		return
	}
	// Order's ReferenceID must match reported ReferenceID
	if ReferenceID != order.PurchaseUnits[0].ReferenceID {
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNKNOWN,
				Msg:    fmt.Sprintf("(Unverified)ReferenceID %s: pp.Order unmatch", ReferenceID),
			},
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "SERVER_PAYPAL_BAD_ORDER",
		})
		return
	}

	// Checkout the Reference from Database
	requestOnRecord, err := sqlwrapper.SelectPaymentRequest(pg.db, pg.orderSqlTable, ReferenceID)
	if err != nil {
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNKNOWN,
				Msg:    fmt.Sprintf("(Verified)ReferenceID %s: Can't check database reference, error: %s", ReferenceID, err),
			},
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "SERVER_BAD_DATABASE",
		})
		return
	}

	// Match paid currency and value
	paypalPricing, err := strconv.ParseFloat(order.PurchaseUnits[0].Amount.Value, 64)
	if err != nil {
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNKNOWN,
				Msg:    fmt.Sprintf("(Verified)ReferenceID %s: failed parsing the amount charged: %s", ReferenceID, err),
			},
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "SERVER_PAYPAL_BAD_ORDER",
		})
		return
	}
	if requestOnRecord.Item.Currency != order.PurchaseUnits[0].Amount.Currency || requestOnRecord.Item.Price != paypalPricing {
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNKNOWN,
				Msg:    fmt.Sprintf("(Verified)ReferenceID %s: payment doesn't match expectation.", ReferenceID),
			},
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "SERVER_NO_MATCHING_ORDER",
		})
		return
	}

	// Only these 2 status means paid
	// TODO: Add Capture attempt upon seeing SAVED
	if order.Status != "APPROVED" && order.Status != "COMPLETED" {
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.UNPAID,
				Msg:    fmt.Sprintf("(Verified)ReferenceID %s: payment is not yet paid.", ReferenceID),
			},
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "PAYMENT_NOT_APPROVED",
		})
		return
	}

	// All verification good. Update the database
	err = sqlwrapper.AppendOrderInfo(pg.db, pg.orderSqlTable, order, CaptureID)
	if err != nil {
		pg.UpdateHandler(
			ReferenceID,
			payment.PaymentResult{
				Status: payment.PAID,
				Unit: payment.PaymentUnit{
					ReferenceID: ReferenceID,
					Currency:    requestOnRecord.Item.Currency,
					Price:       requestOnRecord.Item.Price,
				},
				Msg: fmt.Sprintf("(Verified)ReferenceID %s: Can't update database for confirmed payment, error: %s", ReferenceID, err),
			},
		)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"type":    "payment",
			"message": "SERVER_BAD_DATABASE",
		})
		return
	}

	// All good!
	pg.UpdateHandler(
		ReferenceID,
		payment.PaymentResult{
			Status: payment.PAID,
			Unit: payment.PaymentUnit{
				ReferenceID: ReferenceID,
				Currency:    requestOnRecord.Item.Currency,
				Price:       requestOnRecord.Item.Price,
			},
			Msg: fmt.Sprintf("(Verified)ReferenceID %s: Payment confirmed.", ReferenceID),
		},
	)
	c.JSON(http.StatusBadRequest, gin.H{
		"status":  "success",
		"type":    "payment",
		"message": "PAYMENT_OK",
	})
}
