package creeling;

enum abstract Lifetime(Int) from Int to Int {
  var Borrow;
  var Owned;
  var Leaked;
}
