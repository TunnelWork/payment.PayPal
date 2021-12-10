package paypal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/TunnelWork/Ulysses.Lib/api"
	"github.com/TunnelWork/Ulysses.Lib/payment"
	"github.com/TunnelWork/payment.PayPal/v2/internal/sqlwrapper"
	"github.com/gin-gonic/gin"
	pp "github.com/plutov/paypal/v4"
)

var (
	ErrBadInitConf    error = errors.New("paypal: bad initConf")
	ErrOrderNotPaid   error = errors.New("paypal: order is not in paid state")
	ErrRepeatedRefund error = errors.New("paypal: refund amount exceeds paid amount")
	ErrNoCaptureID    error = errors.New("paypal: no capture ID associated or there was an error when fetching capture ID")

	ExampleInitConf = map[string]string{
		// These 3 needs to be acquired from PayPal developer dashboard
		"clientID": `ABCD`,
		"secretID": `EFGHIJKLMNOPQRST`,
		"apiBase":  `https://api-m.sandbox.paypal.com`, // or https://api-m.paypal.com for PROD

		// A preference for the table name to be used for saving paypal order details.
		// Don't include DB name, for it is protected by *sql.DB.
		"orderSqlTable": `prepaid_paypal_orders_2`, // if unset, will use default value: prepaid_paypal_orders

		// After the payment being executed, user will be 301 to returnURL
		"returnURL": `https://ulysses.tunnel.work/billing.html`, // reserved for future. tmp unused.
	}
)

type PrepaidGateway struct {
	instanceID string

	db            *sql.DB
	orderSqlTable string

	initConf map[string]string // debug only

	// PayPal JS SDK
	sdkScriptURL string

	// plutov/paypal
	client *pp.Client

	//
	onClose func(*gin.Context)

	// Handler func used to notify the Ulysses server
	UpdateHandler *func(referenceID string, newResult payment.PaymentResult)
	callbackBase  string
}

// NewPrepaidGateway() is a payment.PrepaidGatewayGen
func NewPrepaidGateway(db *sql.DB, instanceID string, initConf interface{}) (payment.PrepaidGateway, error) {
	var iConf map[string]string
	var clientID string
	var secretID string
	var apiBase string
	var orderSqlTable string
	var callbackBase string
	var ok bool

	if iConf, ok = initConf.(map[string]string); !ok {
		return nil, ErrBadInitConf
	}

	if clientID, ok = iConf["clientID"]; !ok {
		return nil, ErrBadInitConf
	}
	if secretID, ok = iConf["secretID"]; !ok {
		return nil, ErrBadInitConf
	}
	if apiBase, ok = iConf["apiBase"]; !ok {
		return nil, ErrBadInitConf
	}
	if orderSqlTable, ok = iConf["orderSqlTable"]; !ok {
		orderSqlTable = payment.TblPrefix() + `payment_paypal_prepaid_orders`
	} else if iConf["orderSqlTable"] == "" {
		orderSqlTable = payment.TblPrefix() + `payment_paypal_prepaid_orders`
	}
	if callbackBase, ok = iConf["callbackBase"]; !ok {
		return nil, ErrBadInitConf
	}

	c, err := pp.NewClient(clientID, secretID, apiBase)
	if err != nil {
		return nil, err
	} else {
		_, err := c.GetAccessToken(context.Background())
		if err != nil {
			return nil, err
		}
	}

	if err = sqlwrapper.InitializeTables(db, orderSqlTable); err != nil {
		return nil, err
	}

	var pg PrepaidGateway = PrepaidGateway{
		instanceID:    instanceID,
		db:            db,
		orderSqlTable: orderSqlTable,
		initConf:      iConf,
		sdkScriptURL:  `https://www.paypal.com/sdk/js?client-id=` + clientID + `&currency=`,
		client:        c,
		callbackBase:  callbackBase,
	}
	pg.onClose = pg.handlerPaypalExperienceOnClose

	return &pg, nil
}

// CheckoutForm() is called when frontend requests a Checkout Form to be rendered
func (pg *PrepaidGateway) CheckoutForm(pr payment.PaymentRequest) (formRenderParams map[string]interface{}, err error) {
	// Save the pending order to database
	pr.Item.Price = math.Round(pr.Item.Price*100) / 100

	err = sqlwrapper.PendingOrderID(pg.db, pg.orderSqlTable, pr, PREPAID_GATEWAY)
	if err != nil {
		return nil, err
	}

	OnCloseNotifyURL := fmt.Sprintf("%s/paypal/%s/onClose", pg.callbackBase, pg.instanceID)

	return map[string]interface{}{
		"notify_url": OnCloseNotifyURL,
		"purchase_units": []map[string]interface{}{
			{
				"reference_id": pr.Item.ReferenceID,
				"amount": map[string]interface{}{
					"currency_code": pr.Item.Currency,
					"value":         pr.Item.Price,
				},
			},
		},
		"sdk_url": pg.sdkScriptURL + pr.Item.Currency,
	}, nil
}

// PaymentResult() is called by Ulysses to ACTIVELY verify an order's payment status
// on the contradictory, please see OnStatusChange() where Ulysses waits for
// payment gateway to report the payment result.
func (pg *PrepaidGateway) PaymentResult(referenceID string) (result payment.PaymentResult, err error) {
	orderID, err := sqlwrapper.SelectOrderID(pg.db, pg.orderSqlTable, referenceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return payment.PaymentResult{
				Status: payment.UNPAID,
				Msg:    fmt.Sprintf("ReferenceID %s: No OrderID associated with this reference ID", referenceID),
			}, nil
		}
		return payment.PaymentResult{
			Status: payment.UNKNOWN,
			Msg:    fmt.Sprintf("ReferenceID %s: Can't check with database for OrderID", referenceID),
		}, err
	}
	var order *pp.Order
	order, err = pg.client.GetOrder(context.Background(), orderID)
	if err != nil { // Failed to communicate with PayPal, fail.
		return payment.PaymentResult{
			Status: payment.UNKNOWN,
			Msg:    fmt.Sprintf("ReferenceID %s: Error getting Order from ", referenceID),
		}, err
	}

	var status payment.PaymentStatus
	switch order.Status {
	case "CREATED":
		status = payment.UNPAID
	case "SAVED":
		status = payment.UNPAID
	case "APPROVED":
		status = payment.PAID
	case "COMPLETED":
		status = payment.PAID
	default: // VOIDED or PAYER_ACTION_REQUIRED, we don't handle such conditions
		status = payment.UNKNOWN
	}

	priceFloat, _ := strconv.ParseFloat(order.PurchaseUnits[0].Amount.Value, 64)

	return payment.PaymentResult{
		Status: status,
		Unit: payment.PaymentUnit{
			ReferenceID: order.PurchaseUnits[0].ReferenceID,
			Currency:    order.PurchaseUnits[0].Amount.Currency,
			Price:       priceFloat,
		},
	}, nil
}

// IsRefundable() checks if an order is eligible for at least a partial refund.
func (pg *PrepaidGateway) IsRefundable(referenceID string) bool {
	// 1. Checkout OrderID & CaptureID
	orderID, err := sqlwrapper.SelectOrderID(pg.db, pg.orderSqlTable, referenceID)
	if err != nil {
		return false // Can't check DB -> fail
	}
	captureID, err := sqlwrapper.SelectCaptureID(pg.db, pg.orderSqlTable, referenceID)
	if err != nil || captureID == "" {
		return false // Can't check DB -> fail, no captureID -> fail
	}

	// 2. Check order with PayPal
	var order *pp.Order
	order, err = pg.client.GetOrder(context.Background(), orderID)
	if err != nil { // Failed to communicate with PayPal, fail.
		return false
	}
	if order.Status != "APPROVED" && order.Status != "COMPLETED" {
		return false // Can't refund an unpaid order
	}
	currency := order.PurchaseUnits[0].Amount.Currency
	var amountPaid float64
	amountPaid, _ = strconv.ParseFloat(order.PurchaseUnits[0].Amount.Value, 64)

	// 3. Check if the order has even been completely refunded
	savedCurrency, refunded, err := sqlwrapper.SelectRefunded(pg.db, pg.orderSqlTable, referenceID)
	if err != nil {
		return false // Can't check DB -> fail
	}

	if refunded >= amountPaid || savedCurrency != currency {
		return false // fully refunded or inconsistent result
	}
	return true
}

// Refund the transaction according to a request built by caller
func (pg *PrepaidGateway) Refund(rr payment.RefundRequest) error {
	if rr.Item.Price <= 0 {
		return nil // don't refund at all
	}

	// 1. Checkout OrderID
	orderID, err := sqlwrapper.SelectOrderID(pg.db, pg.orderSqlTable, rr.Item.ReferenceID)
	if err != nil {
		return err // Can't check DB -> fail
	}
	captureID, err := sqlwrapper.SelectCaptureID(pg.db, pg.orderSqlTable, rr.Item.ReferenceID)
	if err != nil || captureID == "" {
		return ErrNoCaptureID // Can't check DB -> fail, no captureID -> fail
	}

	// 2. Check order with PayPal
	var order *pp.Order
	order, err = pg.client.GetOrder(context.Background(), orderID)
	if err != nil { // Failed to communicate with PayPal, fail.
		return err
	}
	if order.Status != "APPROVED" && order.Status != "COMPLETED" {
		return ErrOrderNotPaid // Can't refund an unpaid order
	}
	currency := order.PurchaseUnits[0].Amount.Currency
	var amountPaid float64
	amountPaid, _ = strconv.ParseFloat(order.PurchaseUnits[0].Amount.Value, 64)

	// 3. Check if the order has even been completely refunded
	savedCurrency, refunded, err := sqlwrapper.SelectRefunded(pg.db, pg.orderSqlTable, rr.Item.ReferenceID)
	if err != nil {
		return err // Can't check DB -> fail
	}

	if rr.Item.Currency == "" {
		rr.Item.Currency = savedCurrency
	}

	if refunded+rr.Item.Price > amountPaid || savedCurrency != currency || currency != rr.Item.Currency {
		return ErrRepeatedRefund // fully refunded or inconsistent result
	}

	// Really refund the transaction
	refundResp, refundErr := pg.client.RefundCapture(context.Background(), captureID, pp.RefundCaptureRequest{
		Amount: &pp.Money{
			Currency: rr.Item.Currency,
			Value:    fmt.Sprintf("%.2f", rr.Item.Price),
		},
	})

	if refundErr != nil {
		return refundErr
	}

	if refundResp.Status != "COMPLETED" {
		return fmt.Errorf("paypal: refund status for Reference ID %s is %s, expecting COMPLETED", rr.Item.ReferenceID, refundResp.Status)
	}
	return nil
}

func (pg *PrepaidGateway) OnStatusChange(UpdateHandler *func(referenceID string, newResult payment.PaymentResult)) error {
	pg.UpdateHandler = UpdateHandler

	// https://ulysses.tunnel.work/api/payment/callback/paypal/$id/onClose
	api.CPOST(api.PaymentCallback, fmt.Sprintf("paypal/%s/onClose", pg.instanceID), (*gin.HandlerFunc)(&pg.onClose))

	return nil
}
