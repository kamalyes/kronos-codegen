// 模仿 protoc-gen-go-grpc 的产出 生成器靠这个类型判断契约里有没有对应的
// gRPC 服务，从而决定生成的服务要不要嵌未实现桩
//
// [EN] Imitates protoc-gen-go-grpc's output; the generator uses this type to tell whether
// the contract has a matching gRPC service and therefore whether the generated service
// embeds the unimplemented stub

package contract

// UnimplementedOrderServiceServer 未实现桩
// [EN] The unimplemented stub
type UnimplementedOrderServiceServer struct{}
