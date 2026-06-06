package creeling;

@:structInit
class ASTNode {
  public var kind: HxbNodeKind;
  public var varID: Int;
  public var typeName: String;
  public var methodID: String;
  public var children: Null<Array<ASTNode>> = null;
  public var thenBlock: Null<Array<ASTNode>> = null;
  public var elseBlock: Null<Array<ASTNode>> = null;
}
