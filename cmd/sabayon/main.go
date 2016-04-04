package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/dmathieu/sabayon/certs"
	"github.com/dmathieu/sabayon/heroku"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	var force bool
	flag.BoolVar(&force, "force", false, "Force updating the certificate even if it's not about to expire")
	flag.Parse()

	var domain = os.Getenv("ACME_DOMAIN")
	var email = os.Getenv("ACME_EMAIL")
	var token = os.Getenv("HEROKU_TOKEN")
	var appName = os.Getenv("ACME_APP_NAME")

	herokuClient := heroku.NewClient(nil, token)
	certificates, err := herokuClient.GetSSLCertificates(appName)
	if err != nil {
		log.Fatal(err)
	}

	if len(certificates) > 1 {
		log.Fatalf("Found %d certificate. Can only update one. Nothing done.", len(certificates))
	}

	if !force {
		certExpiration, err := time.Parse(time.RFC3339, certificates[0].SslCert.ExpiresAt)
		if err != nil {
			log.Fatal(err)
		}
		now := time.Now()
		m := now.AddDate(0, +1, 0)

		if certExpiration.After(m) {
			log.Printf("cert.ignore_update expires_at=\"%s\" renew_at=\"%s\"", certExpiration, m)
			return
		}
	}

	log.Printf("cert.create email='%s' domain='%s'", email, domain)

	ce := certs.NewCert(email, domain)
	go ce.Create()

	for {
		select {
		case r := <-ce.ErrChan:
			log.Printf("%s", r)
			return
		case r := <-ce.ChallengeChan:
			log.Printf("cert.validate")

			err := herokuClient.SetConfigVars(appName, r.KeyAuth, r.Token)
			if err != nil {
				log.Fatal(err)
			}

			// Wait for a few seconds so the app can restart
			time.Sleep(5 * time.Second)

			ce.ComChan <- "validate"
		case r := <-ce.ComChan:
			log.Printf("cert.com msg=%s", r)
		case r := <-ce.CertChan:
			log.Printf("cert.created")

			if len(certificates) == 0 {
				err = herokuClient.SetSSLCertificate(appName, r.Certificate, r.PrivateKey)
				if err != nil {
					log.Fatal(err)
				}

				log.Printf("cert.added")
			} else {
				err = herokuClient.UpdateSSLCertificate(appName, certificates[0].Name, r.Certificate, r.PrivateKey)
				if err != nil {
					log.Fatal(err)
				}

				log.Printf("cert.updated")
			}

			return
		}
	}
}