// helper functions
package util

import (
	"strings"

	porkbun "github.com/hoodnoah/porkbun/pkg"
)

type SplitDomain struct {
	Domain    string
	Subdomain string
}

// splits an FQDN into domain, subdomain parts
// and returns them in a labeled struct.
func SplitFQDN(fqdn string) SplitDomain {
	fqdn = strings.TrimSuffix(fqdn, ".")  // remove trailing dot
	parts := strings.SplitN(fqdn, ".", 2) // split domain, subdomain
	if len(parts) < 2 {                   // fallback
		return SplitDomain{
			Domain:    fqdn,
			Subdomain: "",
		}
	}

	return SplitDomain{
		Domain:    parts[1],
		Subdomain: parts[0],
	}
}

// creates a DNS record only after verifying the record does not yet exist.
func CreateDNSRecordByNameTypeIfNotExists(pbClient *porkbun.PorkBun, domain, subdomain, content string) error {
	// fetch existing records
	records, err := pbClient.RetrieveDNSByNameType(domain, subdomain)
	if err != nil {
		return err
	}

	// search for the content in the records; if it exists, skip creation
	for _, record := range records {
		if record.Content == content {
			return nil
		}
	}

	// inferred not to exist; create the record
	return pbClient.CreateDNSByNameType(domain, subdomain, content)
}

// deletes a DNS record only after verifying the record does exist
func DeleteDNSRecordByNameTypeIfExists(pbClient *porkbun.PorkBun, domain, subdomain, content string) error {
	// fetch existing records
	records, err := pbClient.RetrieveDNSByNameType(domain, subdomain)
	if err != nil {
		return err
	}

	// search for the content in the records; if it exists, delete it
	for _, record := range records {
		if record.Content == content {
			return pbClient.DeleteDNSByNameType(domain, subdomain)
		}
	}

	// record never existed
	return nil
}
