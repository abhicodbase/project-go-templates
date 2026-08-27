# gRPC in Go

## What is gRPC?

High-performance RPC framework using **Protocol Buffers** (protobuf) for serialization and **HTTP/2** for transport.

```
REST:   JSON over HTTP/1.1 — text, human-readable, ~500 bytes
gRPC:   Protobuf over HTTP/2 — binary, ~100 bytes, 5-10x faster
```

---

## When to Use gRPC vs REST

| Factor | REST | gRPC |
|--------|------|------|
| **Client type** | Public, browser | Internal service-to-service |
| **Data format** | JSON (human readable) | Binary protobuf |
| **Streaming** | Limited (WebSocket) | Native (4 modes) |
| **Schema** | Optional (OpenAPI) | Mandatory (.proto) |
| **Tooling** | Universal | Needs codegen |
| **Performance** | Good | Excellent |

**Use gRPC for**: Internal microservice communication (Hotel → Booking → Payment)
**Use REST for**: Public APIs, mobile clients, external integrations

---

## Protocol Buffers

```protobuf
// hotel.proto
syntax = "proto3";
package hotel.v1;

option go_package = "github.com/agoda/platform/hotel/v1;hotelv1";

service HotelService {
  rpc GetHotel(GetHotelRequest) returns (Hotel);
  rpc SearchHotels(SearchHotelsRequest) returns (SearchHotelsResponse);
  rpc StreamHotelUpdates(StreamHotelsRequest) returns (stream HotelUpdate);
}

message GetHotelRequest {
  string hotel_id = 1;
}

message Hotel {
  string id = 1;
  string name = 2;
  string city = 3;
  float  rating = 4;
  repeated Room rooms = 5;
}

message Room {
  string type = 1;
  double price_per_night = 2;
  bool   available = 3;
}

message SearchHotelsRequest {
  string city = 1;
  string check_in = 2;   // ISO date "2024-01-15"
  string check_out = 3;
  int32  guests = 4;
  int32  page_size = 5;
  string page_token = 6; // cursor
}

message SearchHotelsResponse {
  repeated Hotel hotels = 1;
  string next_page_token = 2;
}
```

---

## gRPC Server in Go

```go
package main

import (
    "context"
    "fmt"
    "net"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    pb "github.com/agoda/platform/hotel/v1"
)

type hotelServer struct {
    pb.UnimplementedHotelServiceServer
    repo HotelRepository
}

func (s *hotelServer) GetHotel(ctx context.Context, req *pb.GetHotelRequest) (*pb.Hotel, error) {
    if req.HotelId == "" {
        return nil, status.Error(codes.InvalidArgument, "hotel_id is required")
    }

    hotel, err := s.repo.Get(ctx, req.HotelId)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return nil, status.Errorf(codes.NotFound, "hotel %s not found", req.HotelId)
        }
        return nil, status.Errorf(codes.Internal, "failed to get hotel: %v", err)
    }

    return toProto(hotel), nil
}

// Server-side streaming
func (s *hotelServer) StreamHotelUpdates(req *pb.StreamHotelsRequest, stream pb.HotelService_StreamHotelUpdatesServer) error {
    ctx := stream.Context()
    
    updatesCh := s.repo.SubscribeUpdates(ctx, req.CityId)
    for {
        select {
        case update := <-updatesCh:
            if err := stream.Send(&pb.HotelUpdate{Hotel: toProto(update)}); err != nil {
                return err
            }
        case <-ctx.Done():
            return nil
        }
    }
}

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        panic(err)
    }

    s := grpc.NewServer(
        grpc.ChainUnaryInterceptor(
            loggingInterceptor,
            authInterceptor,
            metricsInterceptor,
        ),
    )
    pb.RegisterHotelServiceServer(s, &hotelServer{})
    
    fmt.Println("gRPC server listening on :50051")
    s.Serve(lis)
}
```

---

## gRPC Client in Go

```go
func newHotelClient(addr string) (pb.HotelServiceClient, error) {
    conn, err := grpc.NewClient(addr,
        grpc.WithTransportCredentials(insecure.NewCredentials()), // use TLS in prod!
        grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16*1024*1024)),
    )
    if err != nil {
        return nil, err
    }
    return pb.NewHotelServiceClient(conn), nil
}

func fetchHotel(ctx context.Context, client pb.HotelServiceClient, id string) (*pb.Hotel, error) {
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()

    return client.GetHotel(ctx, &pb.GetHotelRequest{HotelId: id})
}
```

---

## Interceptors (Middleware for gRPC)

```go
// Logging interceptor
func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    start := time.Now()
    resp, err := handler(ctx, req)
    
    code := codes.OK
    if err != nil {
        code = status.Code(err)
    }
    
    log.Info("gRPC call",
        "method", info.FullMethod,
        "code", code,
        "duration", time.Since(start),
    )
    return resp, err
}

// Auth interceptor
func authInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return nil, status.Error(codes.Unauthenticated, "missing metadata")
    }
    
    tokens := md.Get("authorization")
    if len(tokens) == 0 {
        return nil, status.Error(codes.Unauthenticated, "missing authorization token")
    }
    
    claims, err := validateJWT(tokens[0])
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, "invalid token")
    }
    
    ctx = context.WithValue(ctx, userKey, claims.UserID)
    return handler(ctx, req)
}
```

---

## gRPC Error Codes (vs HTTP)

| gRPC Code | HTTP | Meaning |
|-----------|------|---------|
| `OK` | 200 | Success |
| `InvalidArgument` | 400 | Bad request |
| `NotFound` | 404 | Not found |
| `AlreadyExists` | 409 | Conflict |
| `PermissionDenied` | 403 | Forbidden |
| `Unauthenticated` | 401 | Unauthorized |
| `ResourceExhausted` | 429 | Rate limited |
| `Internal` | 500 | Server error |
| `Unavailable` | 503 | Service down |
| `DeadlineExceeded` | 504 | Timeout |

---

## Interview Q&A

**Q: Why would you choose gRPC over REST for internal services?**
> A: Three main reasons: (1) **Performance** — protobuf is 5-10x smaller than JSON and faster to serialize/deserialize; (2) **Streaming** — gRPC natively supports server-side, client-side, and bidirectional streaming over a single HTTP/2 connection, which is great for real-time hotel availability updates; (3) **Strongly typed contracts** — the .proto schema is the API contract, shared between teams, with codegen preventing mismatches. The downside: harder to debug (binary protocol), requires protobuf toolchain, not browser-friendly. For Agoda's internal hotel-to-booking service calls, gRPC is ideal.

**Q: What are the 4 types of gRPC communication?**
> A: (1) Unary — single request, single response (most common); (2) Server streaming — single request, stream of responses (e.g., streaming hotel price updates); (3) Client streaming — stream of requests, single response (e.g., batch uploading hotel photos); (4) Bidirectional streaming — both sides stream simultaneously (e.g., live chat, real-time collaboration). All use the same HTTP/2 connection.
