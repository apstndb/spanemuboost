# SPANner EMUlator BOOtSTrapper

[![Go Reference](https://pkg.go.dev/badge/github.com/apstndb/spanemuboost.svg)](https://pkg.go.dev/github.com/apstndb/spanemuboost)

spanemuboost bootstraps Cloud Spanner Emulator and client with no required configuration using [testcontainers-go](https://github.com/testcontainers/testcontainers-go).

It inspired by `autoConfigEmulator` of:

* [java-spanner-jdbc](https://github.com/googleapis/java-spanner-jdbc?tab=readme-ov-file#commonly-used-properties)
* [java-spanner](https://cloud.google.com/java/docs/reference/google-cloud-spanner/6.62.0/com.google.cloud.spanner.connection.ConnectionOptions.Builder#com_google_cloud_spanner_connection_ConnectionOptions_Builder_setUri_java_lang_String_).