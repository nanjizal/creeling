package creeling;

import haxe.ds.Map;

class FlowContext {
  public var variables = new Map<Int, VariableTrack>();
  public function new(){}
  public function clone(): FlowContext {
      var newCtx = new FlowContext();
      for( k in variables.keys() ){
          var v = variables.get( k );
          newCtx.variables.set( k, { id: v.id, type: v.type, state: v.state } );
      }
      return newCtx;
  }
}
