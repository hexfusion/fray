package e2e_test

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/hexfusion/fray/gen/order"
)

// orderServer implements order.OrderServiceServer with validation.
type orderServer struct {
	order.UnimplementedOrderServiceServer
	mu     sync.RWMutex
	orders map[string]*order.Order
}

func newOrderServer() *orderServer {
	return &orderServer{
		orders: make(map[string]*order.Order),
	}
}

func (s *orderServer) CreateOrder(ctx context.Context, req *order.CreateOrderRequest) (*order.CreateOrderResponse, error) {
	if req.Order == nil {
		return nil, status.Error(codes.InvalidArgument, "order is required")
	}

	// validate the order
	if err := req.Order.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "validation failed: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.orders[req.Order.Id]; exists {
		return nil, status.Error(codes.AlreadyExists, "order already exists")
	}

	s.orders[req.Order.Id] = req.Order
	return &order.CreateOrderResponse{Order: req.Order}, nil
}

func (s *orderServer) GetOrder(ctx context.Context, req *order.GetOrderRequest) (*order.GetOrderResponse, error) {
	// validate the request
	if err := req.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "validation failed: %v", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	o, exists := s.orders[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "order not found")
	}

	return &order.GetOrderResponse{Order: o}, nil
}

func (s *orderServer) UpdateOrder(ctx context.Context, req *order.UpdateOrderRequest) (*order.UpdateOrderResponse, error) {
	if req.Order == nil {
		return nil, status.Error(codes.InvalidArgument, "order is required")
	}

	// validate the new order
	if err := req.Order.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "validation failed: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.orders[req.Order.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "order not found")
	}

	// validate the transition
	if err := req.Order.ValidateUpdate(old); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "transition validation failed: %v", err)
	}

	s.orders[req.Order.Id] = req.Order
	return &order.UpdateOrderResponse{Order: req.Order}, nil
}

func (s *orderServer) ListOrders(ctx context.Context, req *order.ListOrdersRequest) (*order.ListOrdersResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "validation failed: %v", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var orders []*order.Order
	for _, o := range s.orders {
		orders = append(orders, o)
	}

	return &order.ListOrdersResponse{Orders: orders}, nil
}

var _ = Describe("OrderService", func() {
	var (
		server   *grpc.Server
		client   order.OrderServiceClient
		conn     *grpc.ClientConn
		listener net.Listener
	)

	BeforeEach(func() {
		var err error
		listener, err = net.Listen("tcp", "localhost:0")
		Expect(err).NotTo(HaveOccurred())

		server = grpc.NewServer()
		order.RegisterOrderServiceServer(server, newOrderServer())

		go func() {
			_ = server.Serve(listener)
		}()

		conn, err = grpc.NewClient(
			listener.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		Expect(err).NotTo(HaveOccurred())

		client = order.NewOrderServiceClient(conn)
	})

	AfterEach(func() {
		if conn != nil {
			conn.Close()
		}
		if server != nil {
			server.Stop()
		}
		if listener != nil {
			listener.Close()
		}
	})

	Describe("CreateOrder", func() {
		It("creates a valid order", func() {
			ctx := context.Background()
			req := &order.CreateOrderRequest{
				Order: validOrder(),
			}

			resp, err := client.CreateOrder(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Order.Id).To(Equal(req.Order.Id))
		})

		It("rejects order with invalid email", func() {
			ctx := context.Background()
			o := validOrder()
			o.CustomerEmail = "not-an-email"

			_, err := client.CreateOrder(ctx, &order.CreateOrderRequest{Order: o})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(err.Error()).To(ContainSubstring("CustomerEmail"))
		})

		It("rejects order with invalid UUID", func() {
			ctx := context.Background()
			o := validOrder()
			o.Id = "not-a-uuid"

			_, err := client.CreateOrder(ctx, &order.CreateOrderRequest{Order: o})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("rejects order with no items", func() {
			ctx := context.Background()
			o := validOrder()
			o.Items = nil

			_, err := client.CreateOrder(ctx, &order.CreateOrderRequest{Order: o})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(err.Error()).To(ContainSubstring("order must have at least one item"))
		})

		It("rejects order with invalid SKU pattern", func() {
			ctx := context.Background()
			o := validOrder()
			o.Items[0].Sku = "lowercase-sku"

			_, err := client.CreateOrder(ctx, &order.CreateOrderRequest{Order: o})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(err.Error()).To(ContainSubstring("Sku"))
		})

		It("rejects order with quantity too high", func() {
			ctx := context.Background()
			o := validOrder()
			o.Items[0].Quantity = 9999

			_, err := client.CreateOrder(ctx, &order.CreateOrderRequest{Order: o})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})
	})

	Describe("UpdateOrder", func() {
		var existingOrder *order.Order

		BeforeEach(func() {
			ctx := context.Background()
			existingOrder = validOrder()
			_, err := client.CreateOrder(ctx, &order.CreateOrderRequest{Order: existingOrder})
			Expect(err).NotTo(HaveOccurred())
		})

		It("updates order with valid state transition", func() {
			ctx := context.Background()
			updated := cloneOrder(existingOrder)
			updated.Status = order.OrderStatus_CONFIRMED
			updated.Version = 2

			resp, err := client.UpdateOrder(ctx, &order.UpdateOrderRequest{Order: updated})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Order.Status).To(Equal(order.OrderStatus_CONFIRMED))
		})

		It("rejects invalid state transition", func() {
			ctx := context.Background()
			updated := cloneOrder(existingOrder)
			updated.Status = order.OrderStatus_DELIVERED // can't go from PENDING to DELIVERED
			updated.Version = 2

			_, err := client.UpdateOrder(ctx, &order.UpdateOrderRequest{Order: updated})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(err.Error()).To(ContainSubstring("invalid transition"))
		})

		It("rejects changing immutable field", func() {
			ctx := context.Background()
			updated := cloneOrder(existingOrder)
			updated.CustomerId = uuid.New().String()
			updated.Version = 2

			_, err := client.UpdateOrder(ctx, &order.UpdateOrderRequest{Order: updated})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(err.Error()).To(ContainSubstring("immutable"))
		})

		It("rejects decreasing version", func() {
			ctx := context.Background()
			updated := cloneOrder(existingOrder)
			updated.Version = 0 // less than current version 1

			_, err := client.UpdateOrder(ctx, &order.UpdateOrderRequest{Order: updated})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("rejects decreasing total", func() {
			ctx := context.Background()

			// first update to set a higher total
			updated := cloneOrder(existingOrder)
			updated.TotalCents = 5000
			updated.Version = 2
			_, err := client.UpdateOrder(ctx, &order.UpdateOrderRequest{Order: updated})
			Expect(err).NotTo(HaveOccurred())

			// now try to decrease
			decreased := cloneOrder(updated)
			decreased.TotalCents = 1000
			decreased.Version = 3

			_, err = client.UpdateOrder(ctx, &order.UpdateOrderRequest{Order: decreased})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
			Expect(err.Error()).To(ContainSubstring("total can only increase"))
		})
	})

	Describe("GetOrder", func() {
		It("validates request UUID format", func() {
			ctx := context.Background()
			_, err := client.GetOrder(ctx, &order.GetOrderRequest{Id: "not-a-uuid"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("returns not found for missing order", func() {
			ctx := context.Background()
			_, err := client.GetOrder(ctx, &order.GetOrderRequest{Id: uuid.New().String()})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})
	})

	Describe("ListOrders", func() {
		It("validates page size bounds", func() {
			ctx := context.Background()
			_, err := client.ListOrders(ctx, &order.ListOrdersRequest{PageSize: 0})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))

			_, err = client.ListOrders(ctx, &order.ListOrdersRequest{PageSize: 999})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("accepts valid page size", func() {
			ctx := context.Background()
			resp, err := client.ListOrders(ctx, &order.ListOrdersRequest{PageSize: 10})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		})
	})
})

func validOrder() *order.Order {
	return &order.Order{
		Id:            uuid.New().String(),
		CustomerId:    uuid.New().String(),
		CustomerEmail: "customer@example.com",
		Status:        order.OrderStatus_PENDING,
		TotalCents:    1000,
		Version:       1,
		Items: []*order.OrderItem{
			{
				ProductId:  uuid.New().String(),
				Quantity:   2,
				PriceCents: 500,
				Sku:        "SKU-001",
			},
		},
		CreatedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
}

func cloneOrder(o *order.Order) *order.Order {
	items := make([]*order.OrderItem, len(o.Items))
	for i, item := range o.Items {
		items[i] = &order.OrderItem{
			ProductId:  item.ProductId,
			Quantity:   item.Quantity,
			PriceCents: item.PriceCents,
			Sku:        item.Sku,
		}
	}
	return &order.Order{
		Id:            o.Id,
		CustomerId:    o.CustomerId,
		CustomerEmail: o.CustomerEmail,
		Status:        o.Status,
		TotalCents:    o.TotalCents,
		Version:       o.Version,
		Items:         items,
		CreatedAt:     o.CreatedAt,
		UpdatedAt:     o.UpdatedAt,
		ShippingZone:  o.ShippingZone,
	}
}
