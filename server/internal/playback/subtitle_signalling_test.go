package playback

import (
	"strings"
	"testing"
)

func TestHLSSubtitleFlagsFollowSelectionForcedAndAccessibility(t *testing.T) {
	plan := &DeliveryPlan{TextRenditions: []TextRenditionPlan{{Label: "English", Language: "en", Selected: true, Forced: true, HearingImpaired: true}, {Label: "French", Language: "fr"}}}
	master := string(hlsMasterManifest(plan))
	if !strings.Contains(master, `DEFAULT=YES,AUTOSELECT=YES,FORCED=YES,CHARACTERISTICS="public.accessibility.transcribes-spoken-dialog,public.accessibility.describes-music-and-sound"`) {
		t.Fatal(master)
	}
	if !strings.Contains(master, `LANGUAGE="fr",DEFAULT=NO,AUTOSELECT=NO,FORCED=NO`) {
		t.Fatal(master)
	}
}
