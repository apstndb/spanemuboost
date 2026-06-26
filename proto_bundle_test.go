package spanemuboost

import (
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestSetupEmulatorWithClientsProtoBundle(t *testing.T) {
	env := SetupEmulatorWithClients(t,
		WithSetupDDLs([]string{"CREATE PROTO BUNDLE (`examples.shipping.Order`)"}),
		WithSetupFileDescriptorSet(exampleShippingFileDescriptorSet()),
	)

	ctx := t.Context()
	var rows []struct {
		SchemaName  string `spanner:"SCHEMA_NAME"`
		ProtoBundle []byte `spanner:"PROTO_BUNDLE"`
	}
	err := spanner.SelectAll(
		env.Client.Single().Query(ctx, spanner.Statement{
			SQL: "SELECT SCHEMA_NAME, PROTO_BUNDLE FROM INFORMATION_SCHEMA.SCHEMATA",
		}),
		&rows,
		spanner.WithLenient(),
	)
	if err != nil {
		t.Fatal(err)
	}

	var protoBundle []byte
	for _, row := range rows {
		if row.SchemaName == "" && len(row.ProtoBundle) > 0 {
			protoBundle = row.ProtoBundle
			break
		}
	}
	if len(protoBundle) == 0 {
		t.Fatal("SCHEMATA.PROTO_BUNDLE is empty for default schema")
	}
}

func TestSetupClientsProtoBundleWithRawDescriptors(t *testing.T) {
	raw, err := proto.Marshal(exampleShippingFileDescriptorSet())
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	emu := SetupEmulator(t, EnableInstanceAutoConfigOnly())
	clients := SetupClients(t, emu,
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE PROTO BUNDLE (`examples.shipping.Order`)"}),
		WithSetupRawFileDescriptorSet(raw),
	)

	ctx := t.Context()
	var rows []struct {
		SchemaName  string `spanner:"SCHEMA_NAME"`
		ProtoBundle []byte `spanner:"PROTO_BUNDLE"`
	}
	err = spanner.SelectAll(
		clients.Client.Single().Query(ctx, spanner.Statement{
			SQL: "SELECT SCHEMA_NAME, PROTO_BUNDLE FROM INFORMATION_SCHEMA.SCHEMATA",
		}),
		&rows,
		spanner.WithLenient(),
	)
	if err != nil {
		t.Fatal(err)
	}

	var protoBundle []byte
	for _, row := range rows {
		if row.SchemaName == "" && len(row.ProtoBundle) > 0 {
			protoBundle = row.ProtoBundle
			break
		}
	}
	if len(protoBundle) == 0 {
		t.Fatal("SCHEMATA.PROTO_BUNDLE is empty for default schema")
	}
}

func exampleShippingFileDescriptorSet() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Package: proto.String("examples.shipping"),
				EnumType: []*descriptorpb.EnumDescriptorProto{
					{Name: proto.String("ShippingSpeed")},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Order"),
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{Name: proto.String("Status")},
						},
						NestedType: []*descriptorpb.DescriptorProto{
							{Name: proto.String("Address")},
						},
					},
				},
			},
		},
	}
}
