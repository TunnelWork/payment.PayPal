package paypal_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	harpocrates "github.com/TunnelWork/Harpocrates"
	"github.com/TunnelWork/Ulysses.Lib/api"
	"github.com/TunnelWork/Ulysses.Lib/payment"
	paypal "github.com/TunnelWork/payment.PayPal"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

const (
	ContentTypeBinary = "application/octet-stream"
	ContentTypeForm   = "application/x-www-form-urlencoded"
	ContentTypeJSON   = "application/json"
	ContentTypeHTML   = "text/html; charset=utf-8"
	ContentTypeText   = "text/plain; charset=utf-8"
)

const (
	Username = "staging"
	Password = "staging"
	Host     = "127.0.0.1"
	Port     = 3306
	Database = "tmp"
)

var (
	pg           payment.PrepaidGateway
	resultMutex  sync.Mutex = sync.Mutex{}
	globalRefID  string
	latestResult payment.PaymentResult = payment.PaymentResult{
		Msg: "NEW",
	}
	htmlToRender string
)

func stagingDB() (*sql.DB, error) {
	driverName := "mysql"
	// dsn = fmt.Sprintf("user:password@tcp(localhost:5555)/dbname?tls=skip-verify&autocommit=true")
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?loc=Local", Username, Password, Host, Port, Database)

	dsn += "&autocommit=true"

	var db *sql.DB
	var err error
	db, err = sql.Open(driverName, dsn)

	if err != nil {
		return nil, err
	}

	return db, nil
}

func TestInitialization(t *testing.T) {
	db, err := stagingDB()
	if err != nil {
		t.Errorf("stagingDB(): %s", err)
	}

	_, err = paypal.NewPrepaidGateway(db, "staging-test-instance", map[string]string{
		// These 3 needs to be acquired from PayPal developer dashboard
		"clientID": `ARRirLbsebmjl6qOiWuhTQFOhko6HCd-BucbAOnHjtzO5ZZRtG1RxC6SmB18b5fEAmj_oLZTKn8znK1Q`,
		"secretID": `EKdLMEUSbkwUkC3i4MqtbNE5Oq4cSdTOjat4sEV4NiaMHt5LNr25843yy2v90B3jW0iIjEB32eztDP-a`,
		"apiBase":  `https://api-m.sandbox.paypal.com`,

		// A preference for the table name to be used for saving paypal order details.
		// Don't include DB name, for it is protected by *sql.DB.
		"orderSqlTable": `prepaid_paypal_orders_2`, // if unset, will use default value: prepaid_paypal_orders
	})

	if err != nil {
		t.Errorf("paypal.NewPrepaidGateway(): %s", err)
	}
}

func TestCheckout(t *testing.T) {
	db, err := stagingDB()
	if err != nil {
		t.Errorf("stagingDB(): %s", err)
	}

	pg, err = paypal.NewPrepaidGateway(db, "staging-test-instance", map[string]string{
		// These 3 needs to be acquired from PayPal developer dashboard
		"clientID": `ARRirLbsebmjl6qOiWuhTQFOhko6HCd-BucbAOnHjtzO5ZZRtG1RxC6SmB18b5fEAmj_oLZTKn8znK1Q`,
		"secretID": `EKdLMEUSbkwUkC3i4MqtbNE5Oq4cSdTOjat4sEV4NiaMHt5LNr25843yy2v90B3jW0iIjEB32eztDP-a`,
		"apiBase":  `https://api-m.sandbox.paypal.com`,

		// A preference for the table name to be used for saving paypal order details.
		// Don't include DB name, for it is protected by *sql.DB.
		"orderSqlTable": `prepaid_paypal_orders_2`, // if unset, will use default value: prepaid_paypal_orders
	})

	t.Logf("original ptr: %v\n", pg.(*paypal.PrepaidGateway).UpdateHandler)

	pg.OnStatusChange(func(referenceID string, newResult payment.PaymentResult) {
		resultMutex.Lock()
		defer resultMutex.Unlock()
		globalRefID = referenceID
		latestResult = newResult

		t.Logf("Received change on RefID %s\n", referenceID)
		t.Logf("Message: %s\n", newResult.Msg)
	})

	t.Logf("new ptr: %v\n", pg.(*paypal.PrepaidGateway).UpdateHandler)

	t.Logf("try calling ptr...\n")

	if err != nil {
		t.Errorf("paypal.NewPrepaidGateway(): %s\n", err)
	}
	refIdSuffix, err := harpocrates.GetNewWeakPassword(6)
	if err != nil {
		t.Errorf("harpocrates can't give a proper password\n")
	}

	RefID := "StagingTest_" + refIdSuffix

	pr := payment.PaymentRequest{
		Item: payment.PaymentUnit{
			ReferenceID: RefID,
			Currency:    "USD",
			Price:       2.45,
		},
	}
	html, err := pg.CheckoutForm(pr)
	if err != nil {
		t.Errorf("pg.CheckoutForm(): %s\n", err)
	}
	html = strings.Replace(html, "$PAYMENT_CALLBACK_BASE", "http://127.0.0.1:7990/api/payment/callback/", -1)
	html = strings.Replace(html, "$RENDER_PAYMENT_RESULT", "console.log", -1)
	resultMutex.Lock()
	htmlToRender = html
	resultMutex.Unlock()

	router := gin.Default()

	router.GET("orderform", func(c *gin.Context) {
		resultMutex.Lock()
		defer resultMutex.Unlock()
		c.Data(http.StatusOK, ContentTypeHTML, []byte(`<html><head><script src="https://ajax.googleapis.com/ajax/libs/jquery/3.5.1/jquery.min.js"></script><title>Order Form PayPal Gateway</title></head><body>`+htmlToRender+"</body></html>"))
	})

	api.FinalizeGinEngine(router, "api")

	go router.Run(":7990")

	t.Logf("Order Form created and is live at: http://127.0.0.1:7990/orderform")

	// t.Logf("HTML, please try to pay: \n%s\n", html)

	t.Logf("\nWait on status change...\n")
	// Poll the result every 5 seconds
	for {
		resultMutex.Lock()
		if globalRefID == RefID {
			resultMutex.Unlock()
			break
		}
		resultMutex.Unlock()
		time.Sleep(2 * time.Second)
		// t.Logf("new ptr: %v\n", pg.(*paypal.PrepaidGateway).UpdateHandler)
	}

}
