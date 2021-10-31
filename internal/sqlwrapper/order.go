package sqlwrapper

import (
	"database/sql"
	"encoding/json"

	"github.com/TunnelWork/Ulysses.Lib/payment"
	pp "github.com/plutov/paypal/v4"
)

func PendingOrderID(db *sql.DB, tbl string, request payment.PaymentRequest, gatewayType uint) error {
	if db == nil {
		return ErrNilPointer
	}

	// Then, insert the new one
	stmtInsertOrder, err := db.Prepare(`INSERT INTO ` + tbl + ` (
        ReferenceID, 
        GatewayType,
        Currency,
        Total,
        CreatedAt
    ) VALUE(
        ?,
        ?,
        ?,
        ?,
        NOW()
    );`)
	if err != nil {
		return err
	}
	defer stmtInsertOrder.Close()

	_, err = stmtInsertOrder.Exec(
		request.Item.ReferenceID,
		gatewayType,
		request.Item.Currency,
		request.Item.Price,
	)

	return err
}

func AppendOrderInfo(db *sql.DB, tbl string, order *pp.Order, captureID string) error {
	if db == nil || order == nil {
		return ErrNilPointer
	}

	orderID := order.ID

	// bundling is not implemented.
	if len(order.PurchaseUnits) != 1 {
		return ErrBundledPayment
	}

	purchaseUnit := order.PurchaseUnits[0]
	refID := purchaseUnit.ReferenceID
	// currency := purchaseUnit.Amount.Currency
	// paidValue := purchaseUnit.Amount.Value

	// Update order detail
	stmtAppendOrderID, err := db.Prepare(`
    UPDATE ` + tbl + ` 
    SET 
    OrderID = ?,
    OrderDetails = ?, 
	CaptureID = ?,
    ClosedAt = NOW(),
    Active = FALSE 
    WHERE 
    ReferenceID = ? AND Active = TRUE;`)
	if err != nil {
		return err
	}
	defer stmtAppendOrderID.Close()

	orderDetails, err := json.Marshal(*order)
	if err != nil {
		return err
	}

	_, err = stmtAppendOrderID.Exec(
		orderID,
		string(orderDetails),
		captureID,
		refID,
	)

	return err
}

func SelectOrderID(db *sql.DB, tbl string, referenceID string) (orderID string, err error) {
	if db == nil || referenceID == "" {
		return "", ErrNilPointer
	}

	stmtLookupOrderId, err := db.Prepare(`SELECT OrderID FROM ` + tbl + ` WHERE ReferenceID = ?;`)
	if err != nil {
		return "", err
	}
	defer stmtLookupOrderId.Close()
	err = stmtLookupOrderId.QueryRow(referenceID).Scan(&orderID)
	return orderID, err
}

func SelectOrderDetail(db *sql.DB, tbl string, referenceID string) (orderDetailsStr string, err error) {
	if db == nil || referenceID == "" {
		return "", ErrNilPointer
	}

	stmtSelectOrderDetail, err := db.Prepare(`SELECT OrderDetails FROM ` + tbl + ` WHERE ReferenceID = ?;`)
	if err != nil {
		return "", err
	}
	defer stmtSelectOrderDetail.Close()
	err = stmtSelectOrderDetail.QueryRow(referenceID).Scan(&orderDetailsStr)

	return orderDetailsStr, nil
}

func SelectPaymentRequest(db *sql.DB, tbl string, referenceID string) (payment.PaymentRequest, error) {
	if db == nil || referenceID == "" {
		return payment.PaymentRequest{}, ErrNilPointer
	}

	var ReferenceID string
	var Currency string
	var PaidValue float64

	stmtSelectPaymentRequest, err := db.Prepare(`SELECT ReferenceID, Currency, Total FROM ` + tbl + ` WHERE ReferenceID = ?`)
	if err != nil {
		return payment.PaymentRequest{}, err
	}
	defer stmtSelectPaymentRequest.Close()
	err = stmtSelectPaymentRequest.QueryRow(referenceID).Scan(&ReferenceID, &Currency, &PaidValue)
	if err != nil {
		return payment.PaymentRequest{}, err
	}

	return payment.PaymentRequest{
		Item: payment.PaymentUnit{
			ReferenceID: ReferenceID,
			Currency:    Currency,
			Price:       PaidValue,
		},
	}, nil
}

func SelectRefunded(db *sql.DB, tbl string, referenceID string) (string, float64, error) {
	if db == nil || referenceID == "" {
		return "", 0, ErrNilPointer
	}

	var Refunded float64
	var Currency string

	stmtSelectPaymentRequest, err := db.Prepare(`SELECT Currency, Refunded FROM ` + tbl + ` WHERE ReferenceID = ?;`)
	if err != nil {
		return Currency, Refunded, err
	}
	defer stmtSelectPaymentRequest.Close()
	err = stmtSelectPaymentRequest.QueryRow(referenceID).Scan(&Currency, &Refunded)

	return Currency, Refunded, err
}

func SelectCaptureID(db *sql.DB, tbl string, referenceID string) (string, error) {
	if db == nil || referenceID == "" {
		return "", ErrNilPointer
	}

	var CaptureID string

	stmtSelectPaymentRequest, err := db.Prepare(`SELECT CaptureID FROM ` + tbl + ` WHERE ReferenceID = ?;`)
	if err != nil {
		return CaptureID, err
	}
	defer stmtSelectPaymentRequest.Close()
	err = stmtSelectPaymentRequest.QueryRow(referenceID).Scan(&CaptureID)

	return CaptureID, err
}
