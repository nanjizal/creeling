package creeling; 

enum abstract HxbNodeKind( String ) from String to String {
  var NodeAlloc      = "ALLOC";        // Track object heap allocations.
  var NodeVarAccess  = "VAR_ACCESS";   // Capture lifetime boundaries for references.
  var NodeCall       = "METHOD_CALL";  // Identify funcition entries for specialized cloning.
  var NodeIfElse     = "IF_ELSE";      // Process divergent execution flow.
  var NodeBlock      = "BLOCK";        // 
  var NodeFieldStore = "FIELD_STORE";  // Capture escape states when objects move to class properties.
}
