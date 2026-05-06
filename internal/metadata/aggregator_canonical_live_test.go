package metadata

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/vavallee/bindery/internal/metadata/openlibrary"
	"github.com/vavallee/bindery/internal/models"
)

const canonicalPrimaryLiveTestTimeout = 45 * time.Second

type canonicalTortureCase struct {
	id                      string
	titleInputs             []string
	authorInputs            []string
	expectedOpenLibraryWork string
}

func TestLiveAggregatorCanonicalPrimaryBookTortureCorpus(t *testing.T) {
	if os.Getenv("BINDERY_INTEGRATION") == "" {
		t.Skip("skipping live metadata test; set BINDERY_INTEGRATION=1 to run")
	}

	agg := NewAggregator(openlibrary.New())
	for _, tc := range canonicalTortureCorpus() {
		for _, titleInput := range tc.titleInputs {
			for _, authorInput := range tc.authorInputs {
				t.Run(fmt.Sprintf("%s/%s/%s", tc.id, titleInput, authorInput), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), canonicalPrimaryLiveTestTimeout)
					t.Cleanup(cancel)

					source := models.Book{
						Title:            titleInput,
						Author:           &models.Author{Name: authorInput},
						MetadataProvider: "torture-fixture",
					}
					canonical, ok := agg.canonicalPrimaryBook(ctx, "", source)
					if tc.expectedOpenLibraryWork == "" {
						if !ok || canonical == nil {
							t.Logf("canonicalPrimaryBook(%q, %q) did not match; fixture has no expected_openlibrary_work", titleInput, authorInput)
							return
						}
						t.Logf("canonicalPrimaryBook(%q, %q) = %q; fixture has no expected_openlibrary_work", titleInput, authorInput, canonical.ForeignID)
						return
					}
					if !ok {
						t.Fatalf("canonicalPrimaryBook(%q, %q) ok = false, want true", titleInput, authorInput)
					}
					if canonical == nil {
						t.Fatalf("canonicalPrimaryBook(%q, %q) = nil, want OpenLibrary work %s", titleInput, authorInput, tc.expectedOpenLibraryWork)
					}
					if canonical.ForeignID != tc.expectedOpenLibraryWork {
						t.Fatalf("canonical.ForeignID = %q, want %q (title=%q author=%q)", canonical.ForeignID, tc.expectedOpenLibraryWork, titleInput, authorInput)
					}
				})
			}
		}
	}
}

func canonicalTortureCorpus() []canonicalTortureCase {
	return []canonicalTortureCase{
		{
			id: "phm_ascii_subtitle_google_hardcover_ol",
			titleInputs: []string{
				"Project Hail Mary",
				"Project Hail Mary: A Novel",
				"PROJECT HAIL MARY",
			},
			authorInputs: []string{
				"Andy Weir",
				"Weir, Andy",
			},
			expectedOpenLibraryWork: "OL21745884W",
		},
		{
			id: "dune_german_ol_google_dnb_isbn10_x",
			titleInputs: []string{
				"Der Wüstenplanet",
				"Der Wüstenplanet: Science-fiction-Roman",
				"Dune – Der Wüstenplanet",
				"Dune - Der Wüstenplanet.",
				"Dune: Der Wüstenplanet",
			},
			authorInputs: []string{
				"Frank Herbert",
				"Herbert, Frank",
			},
			expectedOpenLibraryWork: "OL893415W",
		},
		{
			id: "dune_german_dnb_new_translation_title_noise",
			titleInputs: []string{
				"Dune – Der Wüstenplanet : Roman",
				"Dune – Der Wüstenplanet: Roman",
				"Der Wüstenplanet - neu übersetzt",
				"Dune der Wüstenplanet Roman",
			},
			authorInputs: []string{
				"Frank Herbert",
				"Herbert, Frank",
			},
			expectedOpenLibraryWork: "OL893415W",
		},
		{
			id: "cien_anos_spanish_accents_bilingual",
			titleInputs: []string{
				"Cien años de soledad",
				"Cien Años de Soledad",
				"Cien años de soledad / One Hundred Years of Solitude",
				"One Hundred Years of Solitude",
			},
			authorInputs: []string{
				"Gabriel García Márquez",
				"Gabriel Garcia Marquez",
				"García Márquez, Gabriel",
				"Garcia Marquez, Gabriel",
			},
			expectedOpenLibraryWork: "OL274505W",
		},
		{
			id: "santi_chinese_simplified_transliteration",
			titleInputs: []string{
				"三体",
				"三体 (sān tǐ)",
				"The Three-Body Problem",
				"The three-body problem",
			},
			authorInputs: []string{
				"刘慈欣",
				"Cixin Liu",
				"Liu Cixin",
			},
			expectedOpenLibraryWork: "OL17267881W",
		},
		{
			id: "huozhe_chinese_short_title",
			titleInputs: []string{
				"活着",
				"To Live",
				"Huo Zhe",
			},
			authorInputs: []string{
				"余华",
				"Yu Hua",
				"Hua Yu",
			},
			expectedOpenLibraryWork: "OL20903102W",
		},
		{
			id: "awlad_haratina_arabic_rtl_translations",
			titleInputs: []string{
				"أولاد حارتنا",
				"Awlād ḥāratinā",
				"Awlad Haretna (Arabic) أولاد حارتنا",
				"Children of the Alley",
				"Children of Gebelaawi",
				"Children of Gebelawi",
			},
			authorInputs: []string{
				"نجيب محفوظ",
				"Naguib Mahfouz",
				"Najib Mahfuz",
			},
			expectedOpenLibraryWork: "OL1599698W",
		},
		{
			id: "master_margarita_cyrillic_translated",
			titleInputs: []string{
				"Мастер и Маргарита",
				"Мастер и Маргарита: roman",
				"The Master and Margarita",
				"Master and Margarita",
			},
			authorInputs: []string{
				"Михаил Афанасьевич Булгаков",
				"Mikhail Bulgakov",
				"Mikhail Afanasyevich Bulgakov",
				"Bulgakov, Mikhail",
			},
			expectedOpenLibraryWork: "OL676009W",
		},
		{
			id: "vorleser_dnb_german_generic_english_title",
			titleInputs: []string{
				"Der Vorleser",
				"Der Vorleser : Roman",
				"The Reader",
			},
			authorInputs: []string{
				"Bernhard Schlink",
				"Schlink, Bernhard",
			},
		},
		{
			id: "kafka_verwandlung_dnb_google_umlaut_translation",
			titleInputs: []string{
				"Die Verwandlung",
				"Die Verwandlung: Erzählung",
				"The Metamorphosis",
			},
			authorInputs: []string{
				"Franz Kafka",
				"Kafka, Franz",
			},
		},
		{
			id: "name_of_the_wind_hardcover_exact_isbn",
			titleInputs: []string{
				"The Name of the Wind",
				"The Name of the Wind: The Kingkiller Chronicle: day one",
				"Name of the Wind, The",
			},
			authorInputs: []string{
				"Patrick Rothfuss",
				"Rothfuss, Patrick",
			},
			expectedOpenLibraryWork: "OL8479867W",
		},
		{
			id: "godel_escher_bach_punctuation_diacritic",
			titleInputs: []string{
				"Gödel, Escher, Bach: an Eternal Golden Braid",
				"Godel, Escher, Bach: An Eternal Golden Braid",
				"Gödel, Escher, Bach",
				"GEB",
			},
			authorInputs: []string{
				"Douglas R. Hofstadter",
				"Douglas Hofstadter",
				"Hofstadter, Douglas R.",
			},
		},
	}
}
