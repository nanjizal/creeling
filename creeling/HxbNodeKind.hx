package creeling; 

enum abstract HxbNodeKind( String ) from String to String {
  var NodeAlloc = "ALLOC";
  var NodeVarAccess = "VAR_ACCESS";
  var NodeCall = "METHOD_CALL";
  var NodeIfElse = "IF_ELSE";
  var NodeBlock = "BLOCK";
  var NodeFieldStore = "FIELD_STORE";
}
