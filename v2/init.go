package paypal

import (
	"github.com/TunnelWork/Ulysses.Lib/payment"
)

func init() {
	var genfunc = NewPrepaidGateway
	payment.RegisterPrepaidGatewayGenerator("paypal", genfunc)
}
