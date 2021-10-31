package sqlwrapper

const (
	ordersTblCreation = `CREATE TABLE IF NOT EXISTS paypal_orders(
        ID INT UNSIGNED NOT NULL AUTO_INCREMENT,
        OrderID VARCHAR(32) NOT NULL DEFAULT '',  
        ReferenceID VARCHAR(32) NOT NULL,  
        GatewayType INT UNSIGNED NOT NULL, 
        Currency VARCHAR(8) NOT NULL DEFAULT 'USD',
        Total FLOAT NOT NULL,
        Refunded FLOAT NOT NULL DEFAULT 0,
        OrderDetails TEXT NOT NULL DEFAULT '',
        CaptureID VARCHAR(32) NOT NULL DEFAULT '',
        CreatedAt DATETIME NOT NULL DEFAULT 0,
        ClosedAt DATETIME NOT NULL DEFAULT 0,
        Active BOOLEAN NOT NULL DEFAULT TRUE,
        PRIMARY KEY (ID),
        INDEX (OrderID),
        UNIQUE (ReferenceID)
    );`
)
