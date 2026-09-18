package photon_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/photon"
)

// feature is one search answer at a point, with the properties given.
func feature(longitude, latitude, properties string) string {
	return `{"type":"Feature","geometry":{"type":"Point","coordinates":[` + longitude + `,` + latitude +
		`]},"properties":{` + properties + `}}`
}

func collection(features ...string) string {
	body := `{"type":"FeatureCollection","features":[`
	for index, entry := range features {
		if index > 0 {
			body += ","
		}
		body += entry
	}

	return body + `]}`
}

func TestSearchAsksForTheQueryNearAPoint(t *testing.T) {
	t.Parallel()
	var path string
	var asked url.Values
	client, _ := serving(t, func(writer http.ResponseWriter, request *http.Request) {
		path, asked = request.URL.Path, request.URL.Query()
		write(t, writer, collection())
	})

	_, err := client.Search(t.Context(), "  Turmberg ", &photon.Near{Latitude: 49, Longitude: 8.4})

	require.NoError(t, err)
	assert.Equal(t, "/api", path)
	assert.Equal(t, url.Values{"q": {"Turmberg"}, "limit": {"6"}, "lat": {"49"}, "lon": {"8.4"}}, asked)
}

func TestSearchAsksWithoutABiasWhereNoneIsGiven(t *testing.T) {
	t.Parallel()
	var asked string
	client, _ := serving(t, func(writer http.ResponseWriter, request *http.Request) {
		asked = request.URL.RawQuery
		write(t, writer, collection())
	})

	_, err := client.Search(t.Context(), "Turmberg", nil)

	require.NoError(t, err)
	assert.NotContains(t, asked, "lat=")
}

func TestSearchAnswersEachPlaceWithItsPointAndSurroundings(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, collection(
			feature("8.4868", "49.0006",
				`"osm_key":"natural","osm_value":"peak","type":"house","name":"Turmberg","district":"Durlach","city":"Karlsruhe","state":"Baden-Württemberg"`),
			feature("8.4121", "49.0094",
				`"osm_key":"building","osm_value":"yes","type":"house","street":"Kaiserstraße","housenumber":"12","city":"Karlsruhe","country":"Deutschland"`),
			feature("8.4017", "48.9937",
				`"osm_key":"railway","osm_value":"station","type":"house","name":"Karlsruhe Hauptbahnhof","district":"Südweststadt","city":"Karlsruhe"`),
			feature("8.4034", "49.0069",
				`"osm_key":"place","osm_value":"city","type":"city","name":"Karlsruhe","county":"Karlsruhe","state":"Baden-Württemberg","country":"Deutschland"`),
			feature("8.3927", "49.0107",
				`"osm_key":"amenity","osm_value":"cafe","type":"house","name":"Café am Kaiserplatz","street":"Kaiserplatz","housenumber":"1"`),
		))
	})

	places, err := client.Search(t.Context(), "karlsruhe", nil)

	require.NoError(t, err)
	assert.Equal(t, []photon.Place{
		{Name: "Turmberg", Context: "Durlach, Karlsruhe", Kind: photon.KindPeak, Latitude: 49.0006, Longitude: 8.4868},
		{Name: "Kaiserstraße 12", Context: "Karlsruhe, Deutschland", Kind: photon.KindAddress, Latitude: 49.0094, Longitude: 8.4121},
		{Name: "Karlsruhe Hauptbahnhof", Context: "Südweststadt, Karlsruhe", Kind: photon.KindStation, Latitude: 48.9937, Longitude: 8.4017},
		{Name: "Karlsruhe", Context: "Baden-Württemberg, Deutschland", Kind: photon.KindSettlement, Latitude: 49.0069, Longitude: 8.4034},
		{Name: "Café am Kaiserplatz", Kind: photon.KindPlace, Latitude: 49.0107, Longitude: 8.3927},
	}, places)
}

func TestSearchNamesACountryByItself(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, collection(
			feature("10.4", "51.1", `"osm_key":"place","osm_value":"country","type":"country","country":"Deutschland"`),
			feature("0", "0", `"osm_key":"highway","osm_value":"path"`),
		))
	})

	places, err := client.Search(t.Context(), "deutschland", nil)

	require.NoError(t, err)
	require.Len(t, places, 1, "an answer with no name at all is left out")
	assert.Equal(t, "Deutschland", places[0].Name)
	assert.Empty(t, places[0].Context)
}

func TestSearchReadsNoMatchAsAnAnswer(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, collection())
	})

	places, err := client.Search(t.Context(), "nowhere at all", nil)

	require.NoError(t, err)
	assert.Empty(t, places)
}

func TestSearchRefusesWhatItCannotAsk(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(http.ResponseWriter, *http.Request) {})

	for name, near := range map[string]*photon.Near{
		"off the earth":      {Latitude: 91, Longitude: 8},
		"past the date line": {Latitude: 49, Longitude: 181},
	} {
		_, err := client.Search(t.Context(), "Turmberg", near)
		var failure *photon.Error
		require.ErrorAs(t, err, &failure, name)
		assert.Equal(t, photon.FailureRefused, failure.Category, name)
	}
	_, err := client.Search(t.Context(), "   ", nil)
	var failure *photon.Error
	require.ErrorAs(t, err, &failure, "an empty query")
	assert.Equal(t, photon.FailureRefused, failure.Category)
}

func TestSearchRefusesAnAnswerWithoutAPoint(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"not json":      `{"features":`,
		"no point":      `{"features":[{"geometry":{"coordinates":[]},"properties":{"name":"Turmberg"}}]}`,
		"off the earth": collection(feature("8", "95", `"name":"Turmberg"`)),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
				write(t, writer, body)
			})

			_, err := client.Search(t.Context(), "Turmberg", nil)

			var failure *photon.Error
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, photon.FailureResponse, failure.Category)
		})
	}
}

func TestSearchCategorisesAFailedRequest(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	})

	_, err := client.Search(t.Context(), "Turmberg", nil)

	var failure *photon.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, photon.FailureGeocoder, failure.Category)
}
