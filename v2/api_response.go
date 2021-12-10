package paypal

import (
	"github.com/TunnelWork/Ulysses.Lib/api"
)

var (
	// success (200 OK)
	PAYMENT_OK = api.MessageResponse(api.SUCCESS, "PAYMENT_OK")

	// cancel (200 OK)
	BUYER_PAYPAL_CANCEL = api.MessageResponse(api.CANCELED, "BUYER_PAYPAL_CANCEL")

	// error
	// 400 Bad Request
	BAD_REQUEST = api.MessageResponse(api.ERROR, "BAD_REQUEST")

	// 503 Service Unavailable
	BUYER_PAYPAL_ERROR = api.MessageResponse(api.ERROR, "BUYER_PAYPAL_ERROR")

	// 409 Conflict
	PAYMENT_NOT_APPROVED = api.MessageResponse(api.ERROR, "PAYMENT_NOT_APPROVED")

	// 500 Internal Server Error
	SERVER_BAD_DATABASE = api.MessageResponse(api.ERROR, "SERVER_BAD_DATABASE")

	// 500 Internal Server Error
	SERVER_PAYPAL_BAD_AUTH = api.MessageResponse(api.ERROR, "SERVER_PAYPAL_BAD_AUTH")

	// PayPal gives a bad order according to the user-reported order ID
	// reason could be:
	// - Can't get such order from PayPal (500)
	// - The order is not intact: payment item or price tainted (400)
	// - ReferenceID not match (400)
	// - Price can't be parsed to float (500)
	SERVER_PAYPAL_BAD_ORDER = api.MessageResponse(api.ERROR, "SERVER_PAYPAL_BAD_ORDER")
)
