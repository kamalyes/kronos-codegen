// 这份文件模仿 protoc 的产出，但刻意不引 protoimpl 与 timestamppb：
// demo 要能在不下载任何第三方依赖的情况下编译
//
// 字段提取只看结构体字段名、protobuf tag 与行尾注释，这三样与真实 .pb.go 一致，
// 所以用它演示与用真实契约演示是等价的
//
// [EN] This file imitates protoc's output while deliberately importing neither protoimpl
// nor timestamppb, so the demo compiles without downloading any third-party dependency
// Field extraction reads only the struct field names, the protobuf tags and the trailing
// comments, all three identical to a real .pb.go, so demonstrating with this is equivalent

package contract

import (
	enums "demo.local/svc/contract/enums"
)

// OrderInfo 订单契约
// [EN] The order contract
type OrderInfo struct {
	OrderId  string            `protobuf:"bytes,1,opt,name=order_id,json=orderId,proto3" json:"order_id,omitempty"`             // 订单ID | [EN] Order ID
	TenantId string            `protobuf:"bytes,2,opt,name=tenant_id,json=tenantId,proto3" json:"tenant_id,omitempty"`          // 租户ID | [EN] Tenant ID
	Title    string            `protobuf:"bytes,3,opt,name=title,proto3" json:"title,omitempty"`                                // 标题 | [EN] Title
	Amount   int64             `protobuf:"varint,4,opt,name=amount,proto3" json:"amount,omitempty"`                             // 金额 | [EN] Amount
	Paid     bool              `protobuf:"varint,5,opt,name=paid,proto3" json:"paid,omitempty"`                                 // 是否已支付 | [EN] Whether paid
	Status   enums.OrderStatus `protobuf:"varint,6,opt,name=status,proto3,enum=demo.enums.OrderStatus" json:"status,omitempty"` // 状态 | [EN] Status
	ItemIds  []string          `protobuf:"bytes,7,rep,name=item_ids,json=itemIds,proto3" json:"item_ids,omitempty"`             // 条目ID列表 | [EN] Item ID list
}
