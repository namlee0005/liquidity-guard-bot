package mongo

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsoncodec"
	"go.mongodb.org/mongo-driver/bson/bsonrw"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	maserrors "liquidity-guard-bot/pkg/errors"
	"github.com/shopspring/decimal"
)

const (
	ConnectTimeout         = 10 * time.Second
	ServerSelectionTimeout = 5 * time.Second
	MaxPoolSize            = 20
	MinPoolSize            = 5
)

var decimalType = reflect.TypeOf(decimal.Decimal{})

// Client wraps *mongo.Client with the configured database name.
type Client struct {
	inner  *mongo.Client
	dbName string
}

func (c *Client) DB() *mongo.Database                       { return c.inner.Database(c.dbName) }
func (c *Client) Collection(name string) *mongo.Collection  { return c.DB().Collection(name) }
func (c *Client) Disconnect(ctx context.Context) error      { return c.inner.Disconnect(ctx) }

func (c *Client) Ping(ctx context.Context) error {
	if err := c.inner.Ping(ctx, nil); err != nil {
		return maserrors.DBError("ping failed", err)
	}
	return nil
}

// NewClient constructs a pooled MongoDB client with Decimal128 codec registered.
// uri example: "mongodb://localhost:27017"
func NewClient(ctx context.Context, uri, dbName string) (*Client, error) {
	registry := buildDecimalRegistry()
	wc := writeconcern.Majority()

	opts := options.Client().
		ApplyURI(uri).
		SetRegistry(registry).
		SetConnectTimeout(ConnectTimeout).
		SetServerSelectionTimeout(ServerSelectionTimeout).
		SetMaxPoolSize(MaxPoolSize).
		SetMinPoolSize(MinPoolSize).
		SetWriteConcern(wc)

	connectCtx, cancel := context.WithTimeout(ctx, ConnectTimeout)
	defer cancel()

	client, err := mongo.Connect(connectCtx, opts)
	if err != nil {
		return nil, maserrors.DBError("failed to connect to MongoDB", err)
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, ServerSelectionTimeout)
	defer pingCancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, maserrors.DBError("MongoDB unreachable after connect", err)
	}

	return &Client{inner: client, dbName: dbName}, nil
}

// buildDecimalRegistry wires shopspring/decimal ↔ BSON Decimal128.
// float64 is NOT registered; accidental float use fails at encode time.
func buildDecimalRegistry() *bsoncodec.Registry {
	rb := bson.NewRegistryBuilder()
	rb.RegisterTypeEncoder(decimalType, bsoncodec.ValueEncoderFunc(encodeDecimal))
	rb.RegisterTypeDecoder(decimalType, bsoncodec.ValueDecoderFunc(decodeDecimal))
	return rb.Build()
}

func encodeDecimal(_ bsoncodec.EncodeContext, vw bsonrw.ValueWriter, val reflect.Value) error {
	d, ok := val.Interface().(decimal.Decimal)
	if !ok {
		return fmt.Errorf("encodeDecimal: expected decimal.Decimal, got %T", val.Interface())
	}
	d128, err := primitive.ParseDecimal128(d.String())
	if err != nil {
		return fmt.Errorf("encodeDecimal: ParseDecimal128: %w", err)
	}
	return vw.WriteDecimal128(d128)
}

func decodeDecimal(_ bsoncodec.DecodeContext, vr bsonrw.ValueReader, val reflect.Value) error {
	if vr.Type() != bsontype.Decimal128 {
		return fmt.Errorf("decodeDecimal: expected Decimal128, got %v", vr.Type())
	}
	d128, err := vr.ReadDecimal128()
	if err != nil {
		return fmt.Errorf("decodeDecimal: ReadDecimal128: %w", err)
	}
	d, err := decimal.NewFromString(d128.String())
	if err != nil {
		return fmt.Errorf("decodeDecimal: parse string: %w", err)
	}
	val.Set(reflect.ValueOf(d))
	return nil
}
