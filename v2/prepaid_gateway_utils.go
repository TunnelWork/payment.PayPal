package paypal

import (
	"database/sql"
	"encoding/json"
)

type PrepaidConfig struct {
	ClientID string `json:"client_id"`
	SecretID string `json:"secret_id"`
	ApiBase  string `json:"api_base"` // https://api-m.sandbox.paypal.com
}

var DefaultPrepaidConfig = PrepaidConfig{
	ClientID: "<YOUR_CLIENT_ID>",
	SecretID: "<YOUR_SECRET>",
	ApiBase:  `https://api-m.sandbox.paypal.com`, // or, if production, `https://api-m.paypal.com`
}

func LoadPrepaidConfig(db *sql.DB, tblPrefix string, instanceID string) (PrepaidConfig, error) {
	var configJson string
	var config PrepaidConfig
	var err error

	stmt, err := db.Prepare(`SELECT config_content FROM ` + tblPrefix + `config WHERE config_name = ?`)
	if err != nil {
		return PrepaidConfig{}, err
	}
	defer stmt.Close()

	err = stmt.QueryRow(`payment_` + instanceID).Scan(&configJson)
	if err != nil {
		if err == sql.ErrNoRows {
			// Insert default config
			stmt2, err := db.Prepare(`INSERT INTO ` + tblPrefix + `config (config_name, config_content) VALUES (?, ?)`)
			if err != nil {
				return PrepaidConfig{}, err
			}
			defer stmt2.Close()

			configJsonByte, err := json.Marshal(DefaultPrepaidConfig)
			if err != nil {
				return PrepaidConfig{}, err
			}
			_, err = stmt2.Exec(`payment_`+instanceID, string(configJsonByte))
			if err != nil {
				return PrepaidConfig{}, err
			}

			return DefaultPrepaidConfig, nil
		} else {
			return PrepaidConfig{}, err
		}
	}

	// unmarshal
	err = json.Unmarshal([]byte(configJson), &config)
	return config, err
}
