package cduschema

import (
	"strings"
	"testing"
)

// Each case is a defect that actually shipped to a browser during the Chudo
// Loyalty build and was only caught by control-cdu's console errors.
func TestValidatePageConfig_RegressionsFromLiveBuild(t *testing.T) {
	cases := []struct {
		name string
		cfg  string
		want string
	}{{
		name: "table head uses id instead of value",
		cfg: `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		 "content":[{"id":"history_tx","class":"table","type":"default",
		 "head":[{"id":"date","title":"Date"}],"body":[]}]}]}]}`,
		want: "head[0].id is not allowed",
	}, {
		name: "table row cells as direct properties",
		cfg: `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		 "content":[{"id":"t","class":"table","type":"default","head":[{"value":"date","title":"D"}],
		 "body":[{"value":"row_0","date":"23.06.2026"}]}]}]}]}`,
		want: "body[0].date is not allowed",
	}, {
		name: "image extra height",
		cfg: `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		 "content":[{"id":"card_qr","class":"image","value":"https://x/y.png",
		 "extra":{"alt":"QR","height":"140"}}]}]}]}`,
		want: "extra.height is not allowed",
	}, {
		name: "empty label value",
		cfg: `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		 "content":[{"id":"session_warning","class":"label","value":""}]}]}]}`,
		want: "must not be an empty string",
	}, {
		name: "empty image src",
		cfg: `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		 "content":[{"id":"promo_img_0","class":"image","value":"","extra":{"alt":"a"}}]}]}]}`,
		want: "must not be an empty string",
	}, {
		name: "stepper extra.direction from the docs",
		cfg: `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		 "content":[{"id":"reg_stepper","class":"stepper","value":"1",
		 "options":[{"value":"1","title":"A"}],"extra":{"direction":"row"}}]}]}]}`,
		want: "extra.direction is not allowed",
	}, {
		name: "image src as a data: URI",
		cfg: `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		 "content":[{"id":"card_qr","class":"image","value":"data:image/gif;base64,R0lGOD","extra":{"alt":"QR"}}]}]}]}`,
		want: "a data: URI is rejected by the image proxy",
	}, {
		name: "stepper option using id instead of value",
		cfg: `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body",
		 "content":[{"id":"reg_stepper","class":"stepper","value":"1",
		 "options":[{"id":"1","title":"A"}]}]}]}]}`,
		want: "options[0].id is not allowed",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateFile("pages/p/config", c.cfg)
			joined := strings.Join(errs, "\n")
			if !strings.Contains(joined, c.want) {
				t.Fatalf("expected an error containing %q, got:\n%s", c.want, joined)
			}
		})
	}
}

// Shapes that are legal must not be reported. The `value` key on an edit is the
// canary for the allOf-union rule: it is declared on Edit-int but not Edit-default.
func TestValidatePageConfig_NoFalsePositives(t *testing.T) {
	cfg := `{"grid":{"type":"one_column"},"forms":[{"id":"login","sections":[{"id":"b","type":"body","content":[
	  {"id":"phone","class":"edit","type":"text","title":"Phone","value":"","required":true,
	   "placeholder":"380","regexp":"^380[0-9]{9}$","helpMsg":"h","styleClass":"c","row":"1","w":"50"},
	  {"id":"pass","class":"edit","type":"password","title":"P","value":""},
	  {"id":"t","class":"table","type":"default","head":[{"value":"date","title":"D"}],
	   "body":"{{rows}}","visibility":"visible"},
	  {"id":"sel","class":"select","type":"autocomplete","value":"","options":"{{opts}}","submitOnChange":true},
	  {"id":"ch","class":"radio","title":"C","value":"","options":[{"value":"a","title":"A"}],
	   "extra":{"direction":"row"}},
	  {"id":"img","class":"image","value":"https://cdn.example/placeholder.png","extra":{"alt":"a"}},
	  {"id":"lbl","class":"label","value":" ","align":"center","styleClass":"hint"},
	  {"id":"cp","class":"copy","value":"123","title":"Copy"},
	  {"id":"btn","class":"button","type":"secondary","title":"Go","extra":{"url":"https://x","target":"_blank"}},
	  {"id":"st","class":"stepper","value":"1","options":[{"value":"1","title":"A","completed":true}]}
	]}]}]}`
	if errs := ValidateFile("pages/p/config", cfg); len(errs) > 0 {
		t.Fatalf("expected no errors, got:\n%s", strings.Join(errs, "\n"))
	}
}

// The swagger under-describes several classes, so the derived union alone rejected
// shapes `cdu-page-protocol.md` §5 documents as real. Each line below is one of
// those rows; the union is widened from the same table (schema.go/applySupplements)
// and TestProbeDocumentedKeysPresentInUnion keeps the two in step.
func TestValidatePageConfig_DocumentedShapesAccepted(t *testing.T) {
	cases := map[string]string{
		"mainMenu.options (§5)":     `{"id":"m","class":"mainMenu","value":"a","options":[{"value":"a","title":"A"}]}`,
		"carousel.items (§5)":       `{"id":"c","class":"carousel","value":"0","items":[{"id":"a","class":"image","value":"https://x/y.png"}],"extra":{"autoplay":true,"interval":3}}`,
		"comments.title (§5)":       `{"id":"cm","class":"comments","value":"x","title":"Chat"}`,
		"timer.extra.duration (§5)": `{"id":"tm","class":"timer","value":"1000","extra":{"duration":10}}`,
		"file.extra urls (§5)":      `{"id":"fl","class":"file","value":"x","extra":{"downloadUrl":"https://x","uploadUrl":"https://y","auth":"t"}}`,
		"upload.extra.compression":  `{"id":"up","class":"upload","value":"x","type":"default","extra":{"compression":0.5}}`,
		"attachment.extra url (§5)": `{"id":"at","class":"attachment","value":"x","extra":{"downloadUrl":"https://x"}}`,
		"base fields on any class":  `{"id":"l","class":"label","value":"x","required":false,"error":false,"errorMsg":"e","submitOnChange":true}`,
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := `{"grid":{"type":"one_column"},"forms":[{"id":"f","sections":[{"id":"s","type":"body","content":[` + item + `]}]}]}`
			if errs := ValidateFile("pages/p/config", cfg); len(errs) > 0 {
				t.Fatalf("documented shape rejected:\n%s", strings.Join(errs, "\n"))
			}
		})
	}
}
